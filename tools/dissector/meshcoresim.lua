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
-- Names, so a filter reads like the network rather than like a database:
--   msim.to_name == "West Lomond"
f.from_name = ProtoField.string("msim.from_name", "From")
f.to_name   = ProtoField.string("msim.to_name",   "Received by")

-- MeshCore's own header byte. Kept minimal deliberately: this duplicates
-- knowledge that also lives in the firmware we link, so it will drift. The
-- honest options are to generate it from one source of truth, or to test it
-- against captures in CI — not to expand it by hand until it looks complete.
f.mc_header = ProtoField.uint8("msim.mc.header", "MeshCore header", base.HEX)

local HDR_LEN = 18

function msim.dissector(buf, pkt, tree)
  if buf:len() < 1 then return 0 end
  pkt.cols.protocol = "MeshcoreSim"
  local t = tree:add(msim, buf())

  -- The live UDP form carries the node names in front of the pseudo-header,
  -- marked by 0xFF - a value the version byte can never take.
  local off = 0
  local from_name, to_name
  if buf(0,1):uint() == 0xFF then
    local n = buf(1,1):uint()
    from_name = buf(2, n):string()
    t:add(f.from_name, buf(2, n))
    local m = buf(2 + n, 1):uint()
    to_name = buf(3 + n, m):string()
    t:add(f.to_name, buf(3 + n, m))
    off = 3 + n + m
  end

  if buf:len() < off + HDR_LEN then return buf:len() end
  local b = buf(off):tvb()

  if b(0,1):uint() ~= 1 then
    t:add_expert_info(PI_PROTOCOL, PI_WARN, "unknown pseudo-header version")
    return buf:len()
  end
  t:add_le(f.version, b(0,1))
  t:add_le(f.outcome, b(1,1))
  t:add_le(f.from,    b(2,2))
  t:add_le(f.to,      b(4,2))
  t:add_le(f.rssi,    b(6,2))
  t:add_le(f.snr,     b(8,2))
  t:add_le(f.freq,    b(10,4))
  t:add_le(f.sf,      b(14,1))
  t:add_le(f.bw,      b(15,2))
  t:add_le(f.cr,      b(17,1))

  local outcome = outcomes[b(1,1):le_uint()] or "unknown"
  pkt.cols.info = string.format("%s -> %s  %s  %.1f dBm",
    from_name or ("node " .. b(2,2):le_uint()),
    to_name or ("node " .. b(4,2):le_uint()),
    outcome, b(6,2):le_int() / 10)

  if b:len() > HDR_LEN then
    local frame = b(HDR_LEN)
    t:add(f.payload, frame)
    t:add(f.mc_header, frame(0,1))
    -- Hand the frame to the MeshCore dissector so the detail shows the route,
    -- the path and the payload rather than a run of bytes.
    Dissector.get("meshcore"):call(frame:tvb(), pkt, tree)
  end
  return buf:len()
end

-- DLT_USER0, for a pcapng file or pipe.
local wtap_encap = DissectorTable.get("wtap_encap")
wtap_encap:add(45, msim)

-- And on loopback UDP, which is how the live view works.
--
-- A pcapng stream carries its section header once, so a reader that attaches
-- after the run started - or a second one - sees a stream it cannot parse and
-- shows nothing at all. Datagrams have no such history: start, stop and restart
-- Wireshark whenever, and it picks up from the next packet.
MSIM_UDP_PORT = 5555
DissectorTable.get("udp.port"):add(MSIM_UDP_PORT, msim)

-- MeshCore's own header, dissected from the frame that follows the
-- pseudo-header.
--
-- The same split as internal/capture/dissect.go, deliberately: two dissectors
-- of one format drift, and the drift is discovered during an argument about a
-- capture. Change them together.
local mc = Proto("meshcore", "MeshCore packet")

local route_names = {
  [0] = "transport flood", [1] = "flood", [2] = "direct", [3] = "transport direct",
}
local payload_names = {
  [0]="request", [1]="response", [2]="text message", [3]="ack", [4]="advert",
  [5]="group text", [6]="group datagram", [7]="anonymous request",
  [8]="returned path", [9]="trace", [10]="multipart", [11]="control",
  [15]="raw custom",
}

local f_route   = ProtoField.uint8("meshcore.route", "Route type", base.DEC, route_names, 0x03)
local f_payload = ProtoField.uint8("meshcore.payload_type", "Payload type", base.DEC, payload_names, 0x3C)
local f_version = ProtoField.uint8("meshcore.version", "Version", base.DEC, nil, 0xC0)
local f_hops    = ProtoField.uint8("meshcore.hops", "Hop count", base.DEC, nil, 0x3F)
local f_hashsz  = ProtoField.uint8("meshcore.path_hash_size", "Path hash size (bytes)", base.DEC, nil, 0xC0)
local f_path    = ProtoField.bytes("meshcore.path", "Path")
local f_hop     = ProtoField.bytes("meshcore.hop", "Hop")
local f_scope   = ProtoField.uint16("meshcore.transport_scope", "Transport scope code", base.HEX)
local f_ret     = ProtoField.uint16("meshcore.transport_return", "Transport return code", base.HEX)
local f_body    = ProtoField.bytes("meshcore.payload", "Payload")
local f_txttype = ProtoField.uint8("meshcore.txt_type", "Text type", base.DEC,
  { [0]="plain", [1]="CLI data", [2]="signed plain" })
local f_chan    = ProtoField.uint8("meshcore.channel_hash", "Channel hash", base.HEX)
local f_stamp   = ProtoField.uint32("meshcore.timestamp", "Sender timestamp", base.DEC)

mc.fields = { f_route, f_payload, f_version, f_hops, f_hashsz, f_path, f_hop,
              f_scope, f_ret, f_body, f_txttype, f_chan, f_stamp }

-- Filterable by all of the above, so Wireshark's own filter bar is the deep
-- analysis surface: `meshcore.payload_type == 4 && meshcoresim.snr < 0` is
-- "adverts that only just arrived", which no UI of ours needs to implement.
function mc.dissector(buf, pinfo, tree)
  if buf:len() < 1 then return 0 end
  local t = tree:add(mc, buf())
  local hdr = buf(0, 1)
  t:add(f_route, hdr)
  t:add(f_payload, hdr)
  t:add(f_version, hdr)

  local route = bit.band(hdr:uint(), 0x03)
  local i = 1

  -- Transport codes, on scoped packets only: the scope the sender used and the
  -- one it wants replies under. Two bytes each, little endian. Worth naming
  -- rather than skipping - on a mesh like ScotMesh the scope *is* the routing,
  -- and "which packets carried this code" is the first question asked of a
  -- capture.
  if route == 0 or route == 3 then
    if buf:len() < i + 4 then return buf:len() end
    t:add_le(f_scope, buf(i, 2))
    t:add_le(f_ret, buf(i + 2, 2))
    i = i + 4
  end
  if buf:len() < i + 1 then return buf:len() end

  -- The path length byte is not a byte count.
  --
  -- Its top two bits are the hash size minus one, and only the low six are the
  -- hop count: path bytes = hops x (size + 1). Reading it as a plain length
  -- mis-parses every packet whose hashes are wider than one byte, which on a
  -- mesh using 2- or 3-byte hashes is all of them - the payload then starts at
  -- the wrong offset and everything after it is nonsense. This is the same bug
  -- that lost a whole region during the ScotMesh study, and Packet.h is the
  -- authority: (path_len >> 6) + 1 and (path_len & 63).
  local plb = buf(i, 1)
  local hash_size = bit.rshift(plb:uint(), 6) + 1
  local hops = bit.band(plb:uint(), 0x3F)
  local path_bytes = hops * hash_size
  t:add(f_hops, plb)
  t:add(f_hashsz, plb):append_text(" (" .. hash_size .. ")")
  i = i + 1
  if buf:len() < i + path_bytes then return buf:len() end

  if path_bytes > 0 then
    local pt = t:add(f_path, buf(i, path_bytes))
    pt:append_text(string.format("  %d hop(s) x %d byte(s)", hops, hash_size))
    -- Each hop on its own, so a path can be read as a route rather than as a
    -- run of bytes.
    for h = 0, hops - 1 do
      pt:add(f_hop, buf(i + h * hash_size, hash_size)):prepend_text("#" .. (h + 1) .. " ")
    end
  end
  i = i + path_bytes

  local ptype = bit.rshift(bit.band(hdr:uint(), 0x3C), 2)
  if buf:len() > i then
    local body = t:add(f_body, buf(i))
    -- Group text is the one worth breaking out: it is what a companion sends
    -- to a hashtag channel, and the channel hash is how a receiver decides
    -- whether the message is even for it.
    if ptype == 5 and buf:len() >= i + 6 then
      body:add(f_chan, buf(i, 1))
      body:add_le(f_stamp, buf(i + 2, 4))
      if buf:len() > i + 6 then
        body:add(f_txttype, buf(i + 1, 1))
      end
    end
  end

  pinfo.cols.info:append(" " .. (payload_names[ptype] or "?"))
  if hops > 0 then
    pinfo.cols.info:append(string.format("  %dh/%db", hops, hash_size))
  end
  if route == 0 or route == 3 then
    pinfo.cols.info:append(" scoped")
  end
  return buf:len()
end
