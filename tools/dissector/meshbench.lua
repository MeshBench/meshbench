-- MeshBench capture metadata, for Wireshark.
--
-- This file is the *metadata* layer only. The MeshCore protocol itself is
-- dissected by meshcore_dissector.lua, which is Aaron Brown's work:
--
--     https://github.com/aaronb/wireshark-meshcore   (GPL-2.0-only)
--
-- Vendored beside this file with its licence, and loaded alongside it. It knows
-- the wire format in far more detail than anything written here would - advert
-- app-data, ack, trace, group data, the lot - and keeping two dissectors for one
-- format only guarantees they drift.
--
-- What this layer adds is what only a simulator knows: which node received the
-- frame, what it then did with it, the true RSSI and SNR, and the node *names*,
-- so a filter reads like the network rather than like a database:
--
--     msim.to_name == "West Lomond"          what did that repeater hear
--     msim.to_name == "West Lomond" && msim.outcome == 1
--                                            ... and failed to decode
--
-- The same transmission is a separate row per receiver, which is the whole
-- point of a merged capture: a packet node A heard and node B did not is the
-- most informative event in a mesh, and no single real receiver can record it.
--
-- Two transports carry it, and both arrive here:
--   * UDP to loopback, for a live view. Datagrams have no history, so Wireshark
--     can be started, stopped and restarted mid-run.
--   * DLT_USER0 in a pcapng file, for one saved afterwards.

local msim = Proto("msim", "MeshBench capture")

local outcomes = {
  [0] = "out of range",
  [1] = "not demodulated",
  [2] = "CRC failed",
  [3] = "dropped by firmware (dedup)",
  [4] = "accepted",
  [5] = "relayed",
}

local f = msim.fields
f.from_name = ProtoField.string("msim.from_name", "From")
f.to_name   = ProtoField.string("msim.to_name",   "Received by")
f.version   = ProtoField.uint8 ("msim.version",   "Header version", base.DEC)
f.outcome   = ProtoField.uint8 ("msim.outcome",   "Outcome",        base.DEC, outcomes)
f.from      = ProtoField.uint16("msim.from",      "From node id",   base.DEC)
f.to        = ProtoField.uint16("msim.to",        "Receiving node id", base.DEC)
f.rssi      = ProtoField.int16 ("msim.rssi",      "RSSI (dBm x10)", base.DEC)
f.snr       = ProtoField.int16 ("msim.snr",       "SNR (dB x10)",   base.DEC)
f.freq      = ProtoField.uint32("msim.freq",      "Frequency (Hz)", base.DEC)
f.sf        = ProtoField.uint8 ("msim.sf",        "Spreading factor", base.DEC)
f.bw        = ProtoField.uint16("msim.bw",        "Bandwidth (kHz)", base.DEC)
f.cr        = ProtoField.uint8 ("msim.cr",        "Coding rate",    base.DEC)
f.crc_ok    = ProtoField.uint8 ("msim.crc_ok",    "CRC OK",         base.DEC)
f.payload   = ProtoField.bytes ("msim.payload",   "MeshCore frame")

-- Fixed part of the pseudo-header, from internal/capture/pcapng.go:
--   version 1, outcome 1, from 2, to 2, rssi 2, snr 2, freq 4, sf 1, bw 2,
--   cr 1, crc_ok 1  =  19
--
-- It was 18 here, which left the CRC-OK byte unread and started the MeshCore
-- frame on it. That byte is 1 for every accepted packet, so every packet in
-- every capture decoded as header 0x01 - "flood, REQ, 1-byte path hash" -
-- which looked like a broken dissector and was arithmetic.
local HDR_LEN = 19

function msim.dissector(buf, pkt, tree)
  if buf:len() < 1 then return 0 end
  pkt.cols.protocol = "MeshBench"
  local t = tree:add(msim, buf())

  -- The live form carries the node names ahead of the fixed header, marked by
  -- 0xFF - a value the version byte can never take, so one dissector reads both
  -- the datagram and the file.
  local off = 0
  local from_name, to_name
  if buf(0, 1):uint() == 0xFF then
    local n = buf(1, 1):uint()
    from_name = buf(2, n):string()
    t:add(f.from_name, buf(2, n))
    local m = buf(2 + n, 1):uint()
    to_name = buf(3 + n, m):string()
    t:add(f.to_name, buf(3 + n, m))
    off = 3 + n + m
  end

  if buf:len() < off + HDR_LEN then return buf:len() end
  local b = buf(off):tvb()
  if b(0, 1):uint() ~= 1 then
    t:add_expert_info(PI_PROTOCOL, PI_WARN, "unknown pseudo-header version")
    return buf:len()
  end

  t:add_le(f.version, b(0, 1))
  t:add_le(f.outcome, b(1, 1))
  t:add_le(f.from,    b(2, 2))
  t:add_le(f.to,      b(4, 2))
  t:add_le(f.rssi,    b(6, 2))
  t:add_le(f.snr,     b(8, 2))
  t:add_le(f.freq,    b(10, 4))
  t:add_le(f.sf,      b(14, 1))
  t:add_le(f.bw,      b(15, 2))
  t:add_le(f.cr,      b(17, 1))
  t:add_le(f.crc_ok,  b(18, 1))

  local outcome = outcomes[b(1, 1):le_uint()] or "unknown"
  pkt.cols.info = string.format("%s -> %s  %s  %.1f dBm",
    from_name or ("node " .. b(2, 2):le_uint()),
    to_name or ("node " .. b(4, 2):le_uint()),
    outcome, b(6, 2):le_int() / 10)

  if b:len() > HDR_LEN then
    local frame = b(HDR_LEN)
    t:add(f.payload, frame)
    -- Hand the frame to the MeshCore protocol dissector. If the vendored one
    -- is not loaded this is simply absent, and the metadata still reads - a
    -- missing plugin should cost detail, not the whole capture.
    local mc = Dissector.get("meshcore")
    if mc then mc:call(frame:tvb(), pkt, tree) end
  end
  return buf:len()
end

-- Live, over loopback UDP.
MSIM_UDP_PORT = 5555
DissectorTable.get("udp.port"):add(MSIM_UDP_PORT, msim)

-- And DLT_USER0, for a saved pcapng.
--
-- The vendored dissector claims the same link type for its own radio layer,
-- which expects a different header. This file sorts after it, so this
-- registration is the one that stands - our captures carry our header.
DissectorTable.get("wtap_encap"):add(45, msim)
