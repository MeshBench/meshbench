-- Wireshark dissector for MeshcoreSim captures.
--
-- Registers against DLT_USER0 (147). Every frame carries a versioned
-- pseudo-header (see internal/capture/pcapng.go) holding what only the
-- simulator knows: which node received it, the true RSSI/SNR, the radio
-- settings, and what the firmware then did with it.
--
-- The receiving node is the important field. The same transmission is a
-- separate row per receiver, so a filter like
--   msim.to == 9 && msim.crc_ok == 0
-- answers "what did node 9 hear but fail to decode" — the question a merged
-- capture exists to answer.

local msim = Proto("msim", "MeshcoreSim")

local outcomes = {
  [0] = "out of range",
  [1] = "not demodulated",
  [2] = "CRC failed",
  [3] = "dropped by firmware (dedup)",
  [4] = "accepted",
  [5] = "relayed",
}

local f = msim.fields
f.version  = ProtoField.uint8 ("msim.version",  "Header version", base.DEC)
f.outcome  = ProtoField.uint8 ("msim.outcome",  "Outcome",        base.DEC, outcomes)
f.from     = ProtoField.uint16("msim.from",     "From node",      base.DEC)
f.to       = ProtoField.uint16("msim.to",       "Receiving node", base.DEC)
f.rssi     = ProtoField.int16 ("msim.rssi",     "RSSI (dBm x10)", base.DEC)
f.snr      = ProtoField.int16 ("msim.snr",      "SNR (dB x10)",   base.DEC)
f.freq     = ProtoField.uint32("msim.freq",     "Frequency (Hz)", base.DEC)
f.sf       = ProtoField.uint8 ("msim.sf",       "Spreading factor", base.DEC)
f.bw       = ProtoField.uint16("msim.bw",       "Bandwidth (kHz)", base.DEC)
f.cr       = ProtoField.uint8 ("msim.cr",       "Coding rate",    base.DEC)
f.crc_ok   = ProtoField.uint8 ("msim.crc_ok",   "CRC OK",         base.DEC)
f.payload  = ProtoField.bytes ("msim.payload",  "MeshCore frame")

-- MeshCore's own header byte. Kept minimal deliberately: this duplicates
-- knowledge that also lives in the firmware we link, so it will drift. The
-- honest options are to generate it from one source of truth, or to test it
-- against captures in CI — not to expand it by hand until it looks complete.
f.mc_header = ProtoField.uint8("msim.mc.header", "MeshCore header", base.HEX)

local HDR_LEN = 18

function msim.dissector(buf, pkt, tree)
  if buf:len() < HDR_LEN then return 0 end
  pkt.cols.protocol = "MeshcoreSim"

  local t = tree:add(msim, buf(), "MeshcoreSim")
  local ver = buf(0,1):le_uint()
  t:add_le(f.version, buf(0,1))
  if ver ~= 1 then
    t:add_expert_info(PI_PROTOCOL, PI_WARN, "unknown pseudo-header version")
    return HDR_LEN
  end

  t:add_le(f.outcome, buf(1,1))
  t:add_le(f.from,    buf(2,2))
  t:add_le(f.to,      buf(4,2))
  t:add_le(f.rssi,    buf(6,2))
  t:add_le(f.snr,     buf(8,2))
  t:add_le(f.freq,    buf(10,4))
  t:add_le(f.sf,      buf(14,1))
  t:add_le(f.bw,      buf(15,2))
  t:add_le(f.cr,      buf(17,1))

  local outcome = outcomes[buf(1,1):le_uint()] or "unknown"
  pkt.cols.info = string.format("node %d -> node %d  %s  %.1f dBm",
    buf(2,2):le_uint(), buf(4,2):le_uint(), outcome, buf(6,2):le_int() / 10)

  if buf:len() > HDR_LEN then
    local frame = buf(HDR_LEN)
    t:add(f.payload, frame)
    t:add(f.mc_header, frame(0,1))
  end
  return buf:len()
end

-- DLT_USER0
local wtap_encap = DissectorTable.get("wtap_encap")
wtap_encap:add(45, msim)
