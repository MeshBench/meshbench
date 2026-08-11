-- MeshCore Protocol Dissector for Wireshark
-- Copyright (C) 2025 Aaron Brown
-- SPDX-License-Identifier: GPL-2.0-only
--
-- Drop this file into ~/.local/lib/wireshark/plugins/ to auto-load.
--
-- Two protocol layers (like 802.11 Radiotap + 802.11):
--   meshcore_radio  – variable-length radio metadata header (version/length/SNR), registered on DLT_USER0
--   meshcore        – actual MeshCore wire protocol, called by meshcore_radio

----------------------------------------
-- Radio metadata layer
----------------------------------------

local meshcore_radio = Proto("meshcore_radio", "MeshCore Radio")

local f_radio_version = ProtoField.uint8("meshcore_radio.version", "Version", base.DEC)
local f_radio_pad = ProtoField.uint8("meshcore_radio.pad", "Pad", base.DEC)
local f_radio_length = ProtoField.uint16("meshcore_radio.length", "Header Length", base.DEC)
local f_radio_snr_raw = ProtoField.int16("meshcore_radio.snr_raw", "SNR raw", base.DEC)

local f_radio_snr = ProtoField.float("meshcore_radio.snr", "SNR (dB)")
local f_radio_payload_len = ProtoField.uint32("meshcore_radio.payload_len", "Payload Length", base.DEC)

meshcore_radio.fields = { f_radio_version, f_radio_pad, f_radio_length, f_radio_snr_raw, f_radio_snr, f_radio_payload_len }

local function format_snr(raw_val)
    return string.format("%.2f dB", raw_val / 100.0)
end

----------------------------------------
-- MeshCore protocol layer
----------------------------------------

local meshcore = Proto("meshcore", "MeshCore Protocol")

local route_types = {
    [0] = "TRANSPORT_FLOOD",
    [1] = "FLOOD",
    [2] = "DIRECT",
    [3] = "TRANSPORT_DIRECT",
}

local payload_types = {
    [0]  = "REQ",
    [1]  = "RESPONSE",
    [2]  = "TXT_MSG",
    [3]  = "ACK",
    [4]  = "ADVERT",
    [5]  = "GRP_TXT",
    [6]  = "GRP_DATA",
    [7]  = "ANON_REQ",
    [8]  = "PATH",
    [9]  = "TRACE",
    [10] = "MULTIPART",
    [11] = "CONTROL",
    [12] = "reserved",
    [13] = "reserved",
    [14] = "reserved",
    [15] = "RAW_CUSTOM",
}

local payload_versions = {
    [0] = "v1",
    [1] = "v2",
    [2] = "v3",
    [3] = "v4",
}

local advert_node_types = {
    [0] = "unknown",
    [1] = "chat_node",
    [2] = "repeater",
    [3] = "room_server",
    [4] = "sensor",
}

local control_sub_types = {
    [8] = "DISCOVER_REQ",
    [9] = "DISCOVER_RESP",
}

-- Header
local f_header        = ProtoField.uint8("meshcore.header", "Header", base.HEX)
local f_route_type    = ProtoField.uint8("meshcore.route_type", "Route Type", base.DEC, route_types, 0x03)
local f_payload_type  = ProtoField.uint8("meshcore.payload_type", "Payload Type", base.DEC, payload_types, 0x3C)
local f_payload_ver   = ProtoField.uint8("meshcore.payload_ver", "Payload Version", base.DEC, payload_versions, 0xC0)

-- Transport codes
local f_transport_code1 = ProtoField.uint16("meshcore.transport_code1", "Transport Code 1", base.HEX)
local f_transport_code2 = ProtoField.uint16("meshcore.transport_code2", "Transport Code 2", base.HEX)

-- Path
local f_path_raw      = ProtoField.uint8("meshcore.path_length", "Path Length (raw byte)", base.HEX)
local f_path_hash_size  = ProtoField.uint8("meshcore.path_hash_size", "Hash Size", base.DEC)
local f_path_hash_count = ProtoField.uint8("meshcore.path_hash_count", "Hash Count", base.DEC)
local f_path          = ProtoField.bytes("meshcore.path", "Path Hashes")

-- Common encrypted payload fields
local f_dest_hash     = ProtoField.uint8("meshcore.dest_hash", "Destination Hash", base.HEX)
local f_src_hash      = ProtoField.uint8("meshcore.src_hash", "Source Hash", base.HEX)
local f_cipher_mac    = ProtoField.uint16("meshcore.cipher_mac", "Cipher MAC", base.HEX)
local f_ciphertext    = ProtoField.bytes("meshcore.ciphertext", "Ciphertext")

-- ACK
local f_ack_checksum  = ProtoField.uint32("meshcore.ack_checksum", "Checksum", base.HEX)

-- ADVERT
local f_adv_pubkey    = ProtoField.bytes("meshcore.advert.pubkey", "Public Key")
local f_adv_timestamp = ProtoField.uint32("meshcore.advert.timestamp", "Timestamp", base.DEC)
local f_adv_signature = ProtoField.bytes("meshcore.advert.signature", "Signature")
local f_adv_flags     = ProtoField.uint8("meshcore.advert.flags", "Flags", base.HEX)
local f_adv_node_type = ProtoField.uint8("meshcore.advert.node_type", "Node Type", base.DEC, advert_node_types, 0x0F)
local f_adv_has_loc   = ProtoField.uint8("meshcore.advert.has_location", "Has Location", base.DEC, nil, 0x10)
local f_adv_has_f1    = ProtoField.uint8("meshcore.advert.has_feature1", "Has Feature 1", base.DEC, nil, 0x20)
local f_adv_has_f2    = ProtoField.uint8("meshcore.advert.has_feature2", "Has Feature 2", base.DEC, nil, 0x40)
local f_adv_has_name  = ProtoField.uint8("meshcore.advert.has_name", "Has Name", base.DEC, nil, 0x80)
local f_adv_lat       = ProtoField.int32("meshcore.advert.latitude", "Latitude (raw)", base.DEC)
local f_adv_lon       = ProtoField.int32("meshcore.advert.longitude", "Longitude (raw)", base.DEC)
local f_adv_feature1  = ProtoField.uint16("meshcore.advert.feature1", "Feature 1", base.HEX)
local f_adv_feature2  = ProtoField.uint16("meshcore.advert.feature2", "Feature 2", base.HEX)
local f_adv_name      = ProtoField.string("meshcore.advert.name", "Node Name")

-- GRP_TXT / GRP_DATA
local f_channel_hash  = ProtoField.uint8("meshcore.channel_hash", "Channel Hash", base.HEX)

-- ANON_REQ
local f_anon_pubkey   = ProtoField.bytes("meshcore.anon_req.pubkey", "Sender Public Key")

-- CONTROL
local f_ctrl_flags    = ProtoField.uint8("meshcore.control.flags", "Control Flags", base.HEX)
local f_ctrl_sub_type = ProtoField.uint8("meshcore.control.sub_type", "Sub Type", base.DEC, control_sub_types, 0xF0)
local f_ctrl_data     = ProtoField.bytes("meshcore.control.data", "Control Data")

-- DISCOVER_REQ fields
local f_disc_req_type_filter = ProtoField.uint8("meshcore.control.disc_req.type_filter", "Type Filter", base.HEX)
local f_disc_req_tag         = ProtoField.uint32("meshcore.control.disc_req.tag", "Tag", base.HEX)
local f_disc_req_since       = ProtoField.uint32("meshcore.control.disc_req.since", "Since", base.DEC)

-- DISCOVER_RESP fields
local f_disc_resp_snr    = ProtoField.int8("meshcore.control.disc_resp.snr", "SNR (raw, *4)", base.DEC)
local f_disc_resp_tag    = ProtoField.uint32("meshcore.control.disc_resp.tag", "Tag", base.HEX)
local f_disc_resp_pubkey = ProtoField.bytes("meshcore.control.disc_resp.pubkey", "Public Key")

-- RAW_CUSTOM
local f_raw_data      = ProtoField.bytes("meshcore.raw_data", "Raw Data")

-- Payload wrapper
local f_payload       = ProtoField.bytes("meshcore.payload_data", "Payload Data")

-- Column-friendly generated fields
local f_col_msg_type  = ProtoField.string("meshcore.msg_type", "Message Type")
local f_col_route     = ProtoField.string("meshcore.route", "Route")
local f_col_src       = ProtoField.string("meshcore.src", "Source")
local f_col_dst       = ProtoField.string("meshcore.dst", "Destination")
local f_col_channel   = ProtoField.string("meshcore.channel", "Channel")
local f_col_node_name = ProtoField.string("meshcore.node_name", "Node Name")
local f_col_hops      = ProtoField.uint8("meshcore.hops", "Hop Count", base.DEC)

-- Decrypted group message fields
local f_grp_channel_name = ProtoField.string("meshcore.grp.channel_name", "Channel Name")
local f_grp_timestamp    = ProtoField.uint32("meshcore.grp.timestamp", "Timestamp", base.DEC)
local f_grp_sender       = ProtoField.string("meshcore.grp.sender", "Sender")
local f_grp_text         = ProtoField.string("meshcore.grp.text", "Text")
local f_grp_plaintext    = ProtoField.string("meshcore.grp.plaintext", "Plaintext")
local f_grp_mac_status   = ProtoField.string("meshcore.grp.mac_status", "MAC Status")

meshcore.fields = {
    f_header, f_route_type, f_payload_type, f_payload_ver,
    f_transport_code1, f_transport_code2,
    f_path_raw, f_path_hash_size, f_path_hash_count, f_path,
    f_dest_hash, f_src_hash, f_cipher_mac, f_ciphertext,
    f_ack_checksum,
    f_adv_pubkey, f_adv_timestamp, f_adv_signature,
    f_adv_flags, f_adv_node_type, f_adv_has_loc, f_adv_has_f1, f_adv_has_f2, f_adv_has_name,
    f_adv_lat, f_adv_lon, f_adv_feature1, f_adv_feature2, f_adv_name,
    f_channel_hash,
    f_anon_pubkey,
    f_ctrl_flags, f_ctrl_sub_type, f_ctrl_data,
    f_disc_req_type_filter, f_disc_req_tag, f_disc_req_since,
    f_disc_resp_snr, f_disc_resp_tag, f_disc_resp_pubkey,
    f_raw_data, f_payload,
    f_col_msg_type, f_col_route, f_col_src, f_col_dst,
    f_col_channel, f_col_node_name, f_col_hops,
    f_grp_channel_name, f_grp_timestamp, f_grp_sender, f_grp_text,
    f_grp_plaintext, f_grp_mac_status,
}

----------------------------------------
-- Pure Lua AES-128 ECB decryption
----------------------------------------

local aes_sbox = {
    [0]=0x63,0x7c,0x77,0x7b,0xf2,0x6b,0x6f,0xc5,0x30,0x01,0x67,0x2b,0xfe,0xd7,0xab,0x76,
    0xca,0x82,0xc9,0x7d,0xfa,0x59,0x47,0xf0,0xad,0xd4,0xa2,0xaf,0x9c,0xa4,0x72,0xc0,
    0xb7,0xfd,0x93,0x26,0x36,0x3f,0xf7,0xcc,0x34,0xa5,0xe5,0xf1,0x71,0xd8,0x31,0x15,
    0x04,0xc7,0x23,0xc3,0x18,0x96,0x05,0x9a,0x07,0x12,0x80,0xe2,0xeb,0x27,0xb2,0x75,
    0x09,0x83,0x2c,0x1a,0x1b,0x6e,0x5a,0xa0,0x52,0x3b,0xd6,0xb3,0x29,0xe3,0x2f,0x84,
    0x53,0xd1,0x00,0xed,0x20,0xfc,0xb1,0x5b,0x6a,0xcb,0xbe,0x39,0x4a,0x4c,0x58,0xcf,
    0xd0,0xef,0xaa,0xfb,0x43,0x4d,0x33,0x85,0x45,0xf9,0x02,0x7f,0x50,0x3c,0x9f,0xa8,
    0x51,0xa3,0x40,0x8f,0x92,0x9d,0x38,0xf5,0xbc,0xb6,0xda,0x21,0x10,0xff,0xf3,0xd2,
    0xcd,0x0c,0x13,0xec,0x5f,0x97,0x44,0x17,0xc4,0xa7,0x7e,0x3d,0x64,0x5d,0x19,0x73,
    0x60,0x81,0x4f,0xdc,0x22,0x2a,0x90,0x88,0x46,0xee,0xb8,0x14,0xde,0x5e,0x0b,0xdb,
    0xe0,0x32,0x3a,0x0a,0x49,0x06,0x24,0x5c,0xc2,0xd3,0xac,0x62,0x91,0x95,0xe4,0x79,
    0xe7,0xc8,0x37,0x6d,0x8d,0xd5,0x4e,0xa9,0x6c,0x56,0xf4,0xea,0x65,0x7a,0xae,0x08,
    0xba,0x78,0x25,0x2e,0x1c,0xa6,0xb4,0xc6,0xe8,0xdd,0x74,0x1f,0x4b,0xbd,0x8b,0x8a,
    0x70,0x3e,0xb5,0x66,0x48,0x03,0xf6,0x0e,0x61,0x35,0x57,0xb9,0x86,0xc1,0x1d,0x9e,
    0xe1,0xf8,0x98,0x11,0x69,0xd9,0x8e,0x94,0x9b,0x1e,0x87,0xe9,0xce,0x55,0x28,0xdf,
    0x8c,0xa1,0x89,0x0d,0xbf,0xe6,0x42,0x68,0x41,0x99,0x2d,0x0f,0xb0,0x54,0xbb,0x16,
}

local aes_inv_sbox = {
    [0]=0x52,0x09,0x6a,0xd5,0x30,0x36,0xa5,0x38,0xbf,0x40,0xa3,0x9e,0x81,0xf3,0xd7,0xfb,
    0x7c,0xe3,0x39,0x82,0x9b,0x2f,0xff,0x87,0x34,0x8e,0x43,0x44,0xc4,0xde,0xe9,0xcb,
    0x54,0x7b,0x94,0x32,0xa6,0xc2,0x23,0x3d,0xee,0x4c,0x95,0x0b,0x42,0xfa,0xc3,0x4e,
    0x08,0x2e,0xa1,0x66,0x28,0xd9,0x24,0xb2,0x76,0x5b,0xa2,0x49,0x6d,0x8b,0xd1,0x25,
    0x72,0xf8,0xf6,0x64,0x86,0x68,0x98,0x16,0xd4,0xa4,0x5c,0xcc,0x5d,0x65,0xb6,0x92,
    0x6c,0x70,0x48,0x50,0xfd,0xed,0xb9,0xda,0x5e,0x15,0x46,0x57,0xa7,0x8d,0x9d,0x84,
    0x90,0xd8,0xab,0x00,0x8c,0xbc,0xd3,0x0a,0xf7,0xe4,0x58,0x05,0xb8,0xb3,0x45,0x06,
    0xd0,0x2c,0x1e,0x8f,0xca,0x3f,0x0f,0x02,0xc1,0xaf,0xbd,0x03,0x01,0x13,0x8a,0x6b,
    0x3a,0x91,0x11,0x41,0x4f,0x67,0xdc,0xea,0x97,0xf2,0xcf,0xce,0xf0,0xb4,0xe6,0x73,
    0x96,0xac,0x74,0x22,0xe7,0xad,0x35,0x85,0xe2,0xf9,0x37,0xe8,0x1c,0x75,0xdf,0x6e,
    0x47,0xf1,0x1a,0x71,0x1d,0x29,0xc5,0x89,0x6f,0xb7,0x62,0x0e,0xaa,0x18,0xbe,0x1b,
    0xfc,0x56,0x3e,0x4b,0xc6,0xd2,0x79,0x20,0x9a,0xdb,0xc0,0xfe,0x78,0xcd,0x5a,0xf4,
    0x1f,0xdd,0xa8,0x33,0x88,0x07,0xc7,0x31,0xb1,0x12,0x10,0x59,0x27,0x80,0xec,0x5f,
    0x60,0x51,0x7f,0xa9,0x19,0xb5,0x4a,0x0d,0x2d,0xe5,0x7a,0x9f,0x93,0xc9,0x9c,0xef,
    0xa0,0xe0,0x3b,0x4d,0xae,0x2a,0xf5,0xb0,0xc8,0xeb,0xbb,0x3c,0x83,0x53,0x99,0x61,
    0x17,0x2b,0x04,0x7e,0xba,0x77,0xd6,0x26,0xe1,0x69,0x14,0x63,0x55,0x21,0x0c,0x7d,
}

local aes_rcon = {
    [1]=0x01, [2]=0x02, [3]=0x04, [4]=0x08, [5]=0x10,
    [6]=0x20, [7]=0x40, [8]=0x80, [9]=0x1b, [10]=0x36,
}

-- GF(2^8) multiplication
local function gf_mul2(a)
    local r = bit.lshift(a, 1)
    if bit.band(a, 0x80) ~= 0 then r = bit.bxor(r, 0x1b) end
    return bit.band(r, 0xff)
end

local function gf_mul(a, b)
    local r = 0
    local aa, bb = a, b
    while bb > 0 do
        if bit.band(bb, 1) ~= 0 then r = bit.bxor(r, aa) end
        aa = gf_mul2(aa)
        bb = bit.rshift(bb, 1)
    end
    return bit.band(r, 0xff)
end

-- Pre-computed GF multiplication tables for InvMixColumns
local gf09, gf0b, gf0d, gf0e = {}, {}, {}, {}
for i = 0, 255 do
    gf09[i] = gf_mul(0x09, i)
    gf0b[i] = gf_mul(0x0b, i)
    gf0d[i] = gf_mul(0x0d, i)
    gf0e[i] = gf_mul(0x0e, i)
end

-- AES-128 key expansion: returns 44 uint32 words (0-indexed)
local function aes128_key_expand(key)
    local w = {}
    for i = 0, 3 do
        w[i] = bit.bor(
            bit.lshift(key[i*4+1], 24), bit.lshift(key[i*4+2], 16),
            bit.lshift(key[i*4+3], 8), key[i*4+4])
    end
    for i = 4, 43 do
        local temp = w[i-1]
        if i % 4 == 0 then
            -- RotWord + SubWord + Rcon
            temp = bit.bor(
                bit.lshift(aes_sbox[bit.band(bit.rshift(temp, 16), 0xff)], 24),
                bit.lshift(aes_sbox[bit.band(bit.rshift(temp, 8), 0xff)], 16),
                bit.lshift(aes_sbox[bit.band(temp, 0xff)], 8),
                aes_sbox[bit.band(bit.rshift(temp, 24), 0xff)])
            temp = bit.bxor(temp, bit.lshift(aes_rcon[i/4], 24))
        end
        w[i] = bit.bxor(w[i-4], temp)
    end
    return w
end

-- AddRoundKey helper (modifies s in-place)
local function aes_add_round_key(s, w, round)
    for col = 0, 3 do
        local word = w[round*4 + col]
        s[col*4]   = bit.bxor(s[col*4],   bit.band(bit.rshift(word, 24), 0xff))
        s[col*4+1] = bit.bxor(s[col*4+1], bit.band(bit.rshift(word, 16), 0xff))
        s[col*4+2] = bit.bxor(s[col*4+2], bit.band(bit.rshift(word, 8), 0xff))
        s[col*4+3] = bit.bxor(s[col*4+3], bit.band(word, 0xff))
    end
end

-- InvShiftRows: row 1 right-shift 1, row 2 right-shift 2, row 3 right-shift 3
local function aes_inv_shift_rows(s)
    local t = s[3*4+1]; s[3*4+1] = s[2*4+1]; s[2*4+1] = s[1*4+1]; s[1*4+1] = s[0*4+1]; s[0*4+1] = t
    t = s[0*4+2]; s[0*4+2] = s[2*4+2]; s[2*4+2] = t; t = s[1*4+2]; s[1*4+2] = s[3*4+2]; s[3*4+2] = t
    t = s[0*4+3]; s[0*4+3] = s[1*4+3]; s[1*4+3] = s[2*4+3]; s[2*4+3] = s[3*4+3]; s[3*4+3] = t
end

-- Decrypt a single AES-128 block (16 bytes in, 16 bytes out)
local function aes128_decrypt_block(block, w)
    -- State: column-major, s[col*4+row] = state[row][col]
    local s = {}
    for i = 0, 15 do s[i] = block[i+1] end

    -- AddRoundKey (round 10)
    aes_add_round_key(s, w, 10)

    -- InvShiftRows + InvSubBytes (undo final encryption round)
    aes_inv_shift_rows(s)
    -- InvSubBytes
    for i = 0, 15 do s[i] = aes_inv_sbox[s[i]] end

    -- Rounds 9 down to 1
    for round = 9, 1, -1 do
        -- AddRoundKey
        aes_add_round_key(s, w, round)
        -- InvMixColumns
        for col = 0, 3 do
            local b = col * 4
            local c0, c1, c2, c3 = s[b], s[b+1], s[b+2], s[b+3]
            s[b]   = bit.bxor(bit.bxor(gf0e[c0], gf0b[c1]), bit.bxor(gf0d[c2], gf09[c3]))
            s[b+1] = bit.bxor(bit.bxor(gf09[c0], gf0e[c1]), bit.bxor(gf0b[c2], gf0d[c3]))
            s[b+2] = bit.bxor(bit.bxor(gf0d[c0], gf09[c1]), bit.bxor(gf0e[c2], gf0b[c3]))
            s[b+3] = bit.bxor(bit.bxor(gf0b[c0], gf0d[c1]), bit.bxor(gf09[c2], gf0e[c3]))
        end
        -- InvShiftRows
        aes_inv_shift_rows(s)
        -- InvSubBytes
        for i = 0, 15 do s[i] = aes_inv_sbox[s[i]] end
    end

    -- AddRoundKey (round 0)
    aes_add_round_key(s, w, 0)

    local result = {}
    for i = 0, 15 do result[i+1] = s[i] end
    return result
end

-- AES-128 ECB decrypt (ciphertext must be multiple of 16 bytes)
local function aes128_ecb_decrypt(expanded_key, ciphertext)
    local plaintext = {}
    for i = 1, #ciphertext, 16 do
        local block = {}
        for j = 0, 15 do block[j+1] = ciphertext[i+j] end
        local dec = aes128_decrypt_block(block, expanded_key)
        for j = 1, 16 do plaintext[#plaintext+1] = dec[j] end
    end
    return plaintext
end

----------------------------------------
-- Pure Lua SHA-256
----------------------------------------

local sha256_k = {
    0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5,
    0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
    0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3,
    0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
    0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc,
    0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
    0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7,
    0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
    0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13,
    0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
    0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3,
    0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
    0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5,
    0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
    0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208,
    0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
}

local sha256_h0 = {
    0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a,
    0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19,
}

local function sha256_rotr(x, n)
    x = bit.band(x, 0xFFFFFFFF)
    return bit.bor(bit.rshift(x, n), bit.lshift(x, 32 - n))
end

local function sha256_add(...)
    local sum = 0
    for i = 1, select('#', ...) do sum = sum + select(i, ...) end
    return bit.band(sum, 0xFFFFFFFF)
end

local function sha256_process_block(H, blk)
    local W = {}
    for i = 1, 16 do
        W[i] = bit.bor(
            bit.lshift(blk[(i-1)*4+1], 24), bit.lshift(blk[(i-1)*4+2], 16),
            bit.lshift(blk[(i-1)*4+3], 8), blk[(i-1)*4+4])
    end
    for i = 17, 64 do
        local w15 = W[i-15]
        local s0 = bit.bxor(sha256_rotr(w15, 7), bit.bxor(sha256_rotr(w15, 18),
                   bit.rshift(bit.band(w15, 0xFFFFFFFF), 3)))
        local w2 = W[i-2]
        local s1 = bit.bxor(sha256_rotr(w2, 17), bit.bxor(sha256_rotr(w2, 19),
                   bit.rshift(bit.band(w2, 0xFFFFFFFF), 10)))
        W[i] = sha256_add(W[i-16], s0, W[i-7], s1)
    end

    local a, b, c, d, e, f, g, h = H[1], H[2], H[3], H[4], H[5], H[6], H[7], H[8]
    for i = 1, 64 do
        local S1 = bit.bxor(sha256_rotr(e, 6), bit.bxor(sha256_rotr(e, 11), sha256_rotr(e, 25)))
        local ch = bit.bxor(bit.band(e, f), bit.band(bit.bnot(e), g))
        local temp1 = sha256_add(h, S1, ch, sha256_k[i], W[i])
        local S0 = bit.bxor(sha256_rotr(a, 2), bit.bxor(sha256_rotr(a, 13), sha256_rotr(a, 22)))
        local maj = bit.bxor(bit.band(a, b), bit.bxor(bit.band(a, c), bit.band(b, c)))
        local temp2 = sha256_add(S0, maj)
        h = g; g = f; f = e; e = sha256_add(d, temp1)
        d = c; c = b; b = a; a = sha256_add(temp1, temp2)
    end
    H[1] = sha256_add(H[1], a); H[2] = sha256_add(H[2], b)
    H[3] = sha256_add(H[3], c); H[4] = sha256_add(H[4], d)
    H[5] = sha256_add(H[5], e); H[6] = sha256_add(H[6], f)
    H[7] = sha256_add(H[7], g); H[8] = sha256_add(H[8], h)
end

local function sha256(data)
    local H = {}
    for i = 1, 8 do H[i] = sha256_h0[i] end
    local len = #data
    local padded = {}
    for i = 1, len do padded[i] = data[i] end
    padded[len + 1] = 0x80
    local pad_len = len + 1
    while pad_len % 64 ~= 56 do pad_len = pad_len + 1; padded[pad_len] = 0 end
    -- Note: bit_len is stored in the low 4 bytes of a 64-bit field. The high
    -- 4 bytes are hardcoded to 0, which truncates messages >512 MB. This is fine
    -- for Wireshark packets (max ~65KB) but would need fixing if reused elsewhere.
    local bit_len = len * 8
    for i = 1, 4 do padded[pad_len + i] = 0 end
    padded[pad_len + 5] = bit.band(bit.rshift(bit_len, 24), 0xff)
    padded[pad_len + 6] = bit.band(bit.rshift(bit_len, 16), 0xff)
    padded[pad_len + 7] = bit.band(bit.rshift(bit_len, 8), 0xff)
    padded[pad_len + 8] = bit.band(bit_len, 0xff)
    pad_len = pad_len + 8
    for i = 1, pad_len, 64 do
        local block = {}
        for j = 1, 64 do block[j] = padded[i + j - 1] end
        sha256_process_block(H, block)
    end
    local hash = {}
    for i = 1, 8 do
        hash[(i-1)*4+1] = bit.band(bit.rshift(H[i], 24), 0xff)
        hash[(i-1)*4+2] = bit.band(bit.rshift(H[i], 16), 0xff)
        hash[(i-1)*4+3] = bit.band(bit.rshift(H[i], 8), 0xff)
        hash[(i-1)*4+4] = bit.band(H[i], 0xff)
    end
    return hash
end

local function hmac_sha256(key, message)
    local block_size = 64
    local k = {}
    if #key > block_size then k = sha256(key)
    else for i = 1, #key do k[i] = key[i] end end
    for i = #k + 1, block_size do k[i] = 0 end

    local inner = {}
    local outer = {}
    for i = 1, block_size do
        inner[i] = bit.bxor(k[i], 0x36)
        outer[i] = bit.bxor(k[i], 0x5c)
    end
    for i = 1, #message do inner[block_size + i] = message[i] end
    local inner_hash = sha256(inner)
    for i = 1, 32 do outer[block_size + i] = inner_hash[i] end
    return sha256(outer)
end

----------------------------------------
-- Well-known channel decryption
----------------------------------------

-- Channel table indexed by hash byte; each entry is a list (for hash collisions)
local well_known_channels = {}

local function register_channel(name, key)
    local hash_bytes = sha256(key)
    local ch_hash = hash_bytes[1]
    -- MeshCore uses the 16-byte channel key for AES-128 encryption, but HMAC-SHA256
    -- expects a 32-byte secret. The protocol zero-pads the 16-byte key to 32 bytes
    -- for the HMAC secret (matching the MeshCore firmware implementation).
    local secret = {}
    for i = 1, 16 do secret[i] = key[i] end
    for i = 17, 32 do secret[i] = 0 end
    local entry = {
        name = name,
        hash = ch_hash,
        key = key,
        secret = secret,
        expanded_key = aes128_key_expand(key),
    }
    if not well_known_channels[ch_hash] then
        well_known_channels[ch_hash] = {}
    end
    table.insert(well_known_channels[ch_hash], entry)
end

-- Public channel PSK — this is a well-known, non-secret key defined by the
-- MeshCore protocol specification.  Anyone can decrypt Public channel traffic;
-- the key is published so that all nodes can interoperate on the default channel.
register_channel("Public",
    {0x8b,0x33,0x87,0xe9,0xc5,0xcd,0xea,0x6a,0xc9,0xe5,0xed,0xba,0xa1,0x15,0xcd,0x72})

-- Hashtag channels: key = SHA256("#name")[0:16]
-- To add a custom hashtag channel, append its name (with leading #) to this list.
local hashtag_names = {"#test","#bot","#testing","#sports","#hamradio","#chat","#jokes","#emergency"}
for _, name in ipairs(hashtag_names) do
    local name_bytes = {}
    for i = 1, #name do name_bytes[i] = string.byte(name, i) end
    local h = sha256(name_bytes)
    local key = {}
    for i = 1, 16 do key[i] = h[i] end
    register_channel(name, key)
end

-- Try to decrypt a group payload. Returns {channel, plaintext} or nil.
local function try_decrypt_grp(ch_hash, mac_b0, mac_b1, ct_bytes)
    local candidates = well_known_channels[ch_hash]
    if not candidates then return nil end
    for _, ch in ipairs(candidates) do
        local hmac_out = hmac_sha256(ch.secret, ct_bytes)
        if hmac_out[1] == mac_b0 and hmac_out[2] == mac_b1 then
            local plaintext = aes128_ecb_decrypt(ch.expanded_key, ct_bytes)
            return { channel = ch, plaintext = plaintext }
        end
    end
    return nil
end

----------------------------------------
-- Payload dissectors
----------------------------------------

local function dissect_advert(tvb, offset, tree, pinfo, remaining)
    if remaining < 100 then
        tree:add(f_payload, tvb(offset, remaining)):append_text(" [too short for ADVERT]")
        return
    end

    local adv_tree = tree:add(meshcore, tvb(offset, remaining), "Payload [ADVERT]")

    adv_tree:add(f_adv_pubkey, tvb(offset, 32))
    offset = offset + 32

    adv_tree:add_le(f_adv_timestamp, tvb(offset, 4))
    local ts_val = tvb(offset, 4):le_uint()
    adv_tree:add(tvb(offset, 4), "Timestamp (date): " .. os.date("!%Y-%m-%d %H:%M:%S UTC", ts_val))
    offset = offset + 4

    adv_tree:add(f_adv_signature, tvb(offset, 64))
    offset = offset + 64

    remaining = remaining - 100

    -- Appdata
    if remaining >= 1 then
        local appdata_tree = adv_tree:add(meshcore, tvb(offset, remaining), "App Data")
        local flags = tvb(offset, 1):uint()
        appdata_tree:add(f_adv_flags, tvb(offset, 1))
        appdata_tree:add(f_adv_node_type, tvb(offset, 1))
        appdata_tree:add(f_adv_has_loc, tvb(offset, 1))
        appdata_tree:add(f_adv_has_f1, tvb(offset, 1))
        appdata_tree:add(f_adv_has_f2, tvb(offset, 1))
        appdata_tree:add(f_adv_has_name, tvb(offset, 1))

        offset = offset + 1
        remaining = remaining - 1

        if bit.band(flags, 0x10) ~= 0 and remaining >= 8 then
            local lat_raw = tvb(offset, 4):le_int()
            local lon_raw = tvb(offset + 4, 4):le_int()
            appdata_tree:add_le(f_adv_lat, tvb(offset, 4)):append_text(
                string.format(" (%.6f°)", lat_raw / 1000000.0))
            appdata_tree:add_le(f_adv_lon, tvb(offset + 4, 4)):append_text(
                string.format(" (%.6f°)", lon_raw / 1000000.0))
            offset = offset + 8
            remaining = remaining - 8
        end

        if bit.band(flags, 0x20) ~= 0 and remaining >= 2 then
            appdata_tree:add_le(f_adv_feature1, tvb(offset, 2))
            offset = offset + 2
            remaining = remaining - 2
        end

        if bit.band(flags, 0x40) ~= 0 and remaining >= 2 then
            appdata_tree:add_le(f_adv_feature2, tvb(offset, 2))
            offset = offset + 2
            remaining = remaining - 2
        end

        if bit.band(flags, 0x80) ~= 0 and remaining > 0 then
            local name_str = tvb(offset, remaining):string()
            appdata_tree:add(f_adv_name, tvb(offset, remaining))
            pinfo.cols.info:append(" '" .. name_str .. "'")

            -- Column field
            local col_name = tree:add(f_col_node_name, tvb(offset, remaining), name_str)
            col_name:set_generated()
            col_name:set_hidden()

            pinfo.cols.src = name_str
            pinfo.cols.dst = "broadcast"
        end
    end
end

local function dissect_ack(tvb, offset, tree, remaining)
    if remaining < 4 then
        tree:add(f_payload, tvb(offset, remaining)):append_text(" [too short for ACK]")
        return
    end
    local ack_tree = tree:add(meshcore, tvb(offset, remaining), "Payload [ACK]")
    ack_tree:add_le(f_ack_checksum, tvb(offset, 4))
end

local function dissect_encrypted(tvb, offset, tree, pinfo, remaining, label)
    if remaining < 4 then
        tree:add(f_payload, tvb(offset, remaining)):append_text(" [too short]")
        return
    end
    local enc_tree = tree:add(meshcore, tvb(offset, remaining), "Payload [" .. label .. "]")
    enc_tree:add(f_dest_hash, tvb(offset, 1))
    enc_tree:add(f_src_hash, tvb(offset + 1, 1))
    enc_tree:add_le(f_cipher_mac, tvb(offset + 2, 2))
    if remaining > 4 then
        enc_tree:add(f_ciphertext, tvb(offset + 4, remaining - 4))
    end

    -- Column fields
    local dst_str = string.format("0x%02x", tvb(offset, 1):uint())
    local src_str = string.format("0x%02x", tvb(offset + 1, 1):uint())
    local col_dst = tree:add(f_col_dst, tvb(offset, 1), dst_str)
    col_dst:set_generated()
    col_dst:set_hidden()
    local col_src = tree:add(f_col_src, tvb(offset + 1, 1), src_str)
    col_src:set_generated()
    col_src:set_hidden()

    pinfo.cols.src = src_str
    pinfo.cols.dst = dst_str
end

local function dissect_grp(tvb, offset, tree, pinfo, remaining, label)
    if remaining < 3 then
        tree:add(f_payload, tvb(offset, remaining)):append_text(" [too short]")
        return
    end
    local grp_tree = tree:add(meshcore, tvb(offset, remaining), "Payload [" .. label .. "]")
    local ch = tvb(offset, 1):uint()
    grp_tree:add(f_channel_hash, tvb(offset, 1))
    grp_tree:add_le(f_cipher_mac, tvb(offset + 1, 2))

    local ct_offset = offset + 3
    local ct_len = remaining - 3

    if ct_len > 0 then
        grp_tree:add(f_ciphertext, tvb(ct_offset, ct_len))
    end

    -- Attempt decryption for well-known channels
    local decrypted = nil
    if ct_len > 0 and ct_len % 16 == 0 then
        local mac_b0 = tvb(offset + 1, 1):uint()
        local mac_b1 = tvb(offset + 2, 1):uint()
        local ct_ba = tvb(ct_offset, ct_len):bytes()
        local ct_bytes = {}
        for i = 0, ct_ba:len() - 1 do ct_bytes[i+1] = ct_ba:get_index(i) end
        decrypted = try_decrypt_grp(ch, mac_b0, mac_b1, ct_bytes)
    elseif ct_len > 0 and well_known_channels[ch] then
        grp_tree:add_expert_info(PI_DECRYPTION, PI_WARN,
            "Known channel hash but ciphertext length (" .. ct_len .. ") is not a multiple of 16")
    end

    if decrypted then
        local ch_name = decrypted.channel.name
        local pt = decrypted.plaintext

        local name_item = grp_tree:add(f_grp_channel_name, tvb(offset, 1), ch_name)
        name_item:set_generated()
        local mac_item = grp_tree:add(f_grp_mac_status, tvb(offset + 1, 2), "OK")
        mac_item:set_generated()

        -- Build a new Tvb from the decrypted bytes (like TLS/WPA2 decryption)
        local ba = ByteArray.new()
        ba:set_size(#pt)
        for i = 1, #pt do ba:set_index(i - 1, pt[i]) end
        local dec_tvb = ba:tvb("Decrypted Group Payload")

        -- Strip trailing zero-padding to find content length
        local content_len = #pt
        while content_len > 4 and pt[content_len] == 0 do content_len = content_len - 1 end

        local dec_tree = grp_tree:add(meshcore, dec_tvb(), "Decrypted Payload")

        if content_len >= 4 then
            dec_tree:add_le(f_grp_timestamp, dec_tvb(0, 4))
                :append_text(" (" .. os.date("!%Y-%m-%d %H:%M:%S UTC", dec_tvb(0, 4):le_uint()) .. ")")

            if content_len > 4 then
                -- Build display string filtering out null bytes
                local msg_chars = {}
                for i = 5, content_len do
                    if pt[i] ~= 0 then
                        msg_chars[#msg_chars+1] = string.char(pt[i])
                    end
                end
                local msg = table.concat(msg_chars)
                if #msg > 0 then
                    dec_tree:add(f_grp_plaintext, dec_tvb(4, content_len - 4), msg)

                    -- Parse "sender: text" format for GRP_TXT
                    local colon_pos = msg:find(": ", 1, true)
                    if colon_pos and label == "GRP_TXT" then
                        local sender = msg:sub(1, colon_pos - 1)
                        local text = msg:sub(colon_pos + 2)
                        local sender_item = dec_tree:add(f_grp_sender, dec_tvb(4, content_len - 4), sender)
                        sender_item:set_generated()
                        if #text > 0 then
                            local text_item = dec_tree:add(f_grp_text, dec_tvb(4, content_len - 4), text)
                            text_item:set_generated()
                        end
                        pinfo.cols.src = sender
                    end

                    pinfo.cols.info:append(" '" .. msg .. "'")
                end
            end
        end

        pinfo.cols.dst = ch_name
        pinfo.cols.info:append(string.format(" ch=0x%02x (%s)", ch, ch_name))
    else
        pinfo.cols.dst = string.format("ch:0x%02x", ch)
        pinfo.cols.info:append(string.format(" ch=0x%02x", ch))
    end

    -- Column field
    local col_ch = tree:add(f_col_channel, tvb(offset, 1), string.format("0x%02x", ch))
    col_ch:set_generated()
    col_ch:set_hidden()
end

local function dissect_anon_req(tvb, offset, tree, pinfo, remaining)
    if remaining < 35 then
        tree:add(f_payload, tvb(offset, remaining)):append_text(" [too short for ANON_REQ]")
        return
    end
    local anon_tree = tree:add(meshcore, tvb(offset, remaining), "Payload [ANON_REQ]")
    anon_tree:add(f_dest_hash, tvb(offset, 1))
    anon_tree:add(f_anon_pubkey, tvb(offset + 1, 32))
    anon_tree:add_le(f_cipher_mac, tvb(offset + 33, 2))
    if remaining > 35 then
        anon_tree:add(f_ciphertext, tvb(offset + 35, remaining - 35))
    end

    -- Column field
    local dst_str = string.format("0x%02x", tvb(offset, 1):uint())
    local col_dst = tree:add(f_col_dst, tvb(offset, 1), dst_str)
    col_dst:set_generated()
    col_dst:set_hidden()

    pinfo.cols.src = "anon"
    pinfo.cols.dst = dst_str
end

local function dissect_control(tvb, offset, tree, pinfo, remaining)
    if remaining < 1 then return end
    local ctrl_tree = tree:add(meshcore, tvb(offset, remaining), "Payload [CONTROL]")
    local flags = tvb(offset, 1):uint()
    local sub_type = bit.rshift(bit.band(flags, 0xF0), 4)

    ctrl_tree:add(f_ctrl_flags, tvb(offset, 1))
    ctrl_tree:add(f_ctrl_sub_type, tvb(offset, 1))

    local sub_name = control_sub_types[sub_type] or string.format("0x%x", sub_type)
    pinfo.cols.info:append(" " .. sub_name)

    offset = offset + 1
    remaining = remaining - 1

    if sub_type == 8 and remaining >= 5 then
        ctrl_tree:add(f_disc_req_type_filter, tvb(offset, 1))
        ctrl_tree:add_le(f_disc_req_tag, tvb(offset + 1, 4))
        if remaining >= 9 then
            ctrl_tree:add_le(f_disc_req_since, tvb(offset + 5, 4))
        end
    elseif sub_type == 9 and remaining >= 5 then
        ctrl_tree:add(f_disc_resp_snr, tvb(offset, 1))
        ctrl_tree:add_le(f_disc_resp_tag, tvb(offset + 1, 4))
        if remaining > 5 then
            ctrl_tree:add(f_disc_resp_pubkey, tvb(offset + 5, remaining - 5))
        end
    elseif remaining > 0 then
        ctrl_tree:add(f_ctrl_data, tvb(offset, remaining))
    end
end

----------------------------------------
-- MeshCore dissector (wire protocol)
----------------------------------------

function meshcore.dissector(tvb, pinfo, tree)
    local offset = 0
    local buf_len = tvb:len()

    pinfo.cols.protocol = "MeshCore"

    local subtree = tree:add(meshcore, tvb(), "MeshCore Protocol")

    if buf_len < 2 then return end  -- need at least header + path_length

    -- Header byte
    local header_byte = tvb(offset, 1):uint()
    local route_type = bit.band(header_byte, 0x03)
    local payload_type = bit.rshift(bit.band(header_byte, 0x3C), 2)
    local payload_ver = bit.rshift(bit.band(header_byte, 0xC0), 6)

    local hdr_tree = subtree:add(f_header, tvb(offset, 1))
    hdr_tree:add(f_route_type, tvb(offset, 1))
    hdr_tree:add(f_payload_type, tvb(offset, 1))
    hdr_tree:add(f_payload_ver, tvb(offset, 1))
    offset = offset + 1

    -- Transport codes (present for route types 0 and 3)
    if route_type == 0 or route_type == 3 then
        if buf_len - offset < 4 then return end
        local tc_tree = subtree:add(meshcore, tvb(offset, 4), "Transport Codes")
        tc_tree:add_le(f_transport_code1, tvb(offset, 2))
        tc_tree:add_le(f_transport_code2, tvb(offset + 2, 2))
        offset = offset + 4
    end

    -- Path
    if buf_len - offset < 1 then return end
    local path_len_byte = tvb(offset, 1):uint()
    local hash_count = bit.band(path_len_byte, 0x3F)
    local hash_size = bit.rshift(path_len_byte, 6) + 1
    local path_bytes = hash_count * hash_size

    local path_tree = subtree:add(meshcore, tvb(offset, 1 + path_bytes),
        string.format("Path (hash_size=%d, count=%d, %d bytes)", hash_size, hash_count, path_bytes))
    path_tree:add(f_path_raw, tvb(offset, 1))
    local pi_hs = path_tree:add(f_path_hash_size, tvb(offset, 1), hash_size)
    pi_hs:set_generated()
    local pi_hc = path_tree:add(f_path_hash_count, tvb(offset, 1), hash_count)
    pi_hc:set_generated()
    offset = offset + 1

    if path_bytes > 0 then
        if buf_len - offset < path_bytes then return end
        path_tree:add(f_path, tvb(offset, path_bytes))
        offset = offset + path_bytes
    end

    -- Payload
    local remaining = buf_len - offset

    -- Build info column
    local pt_str = payload_types[payload_type] or "UNKNOWN"
    local rt_str = route_types[route_type] or "UNKNOWN"
    local pv_str = payload_versions[payload_ver] or "?"
    pinfo.cols.info = string.format("MeshCore %s %s %s", pt_str, rt_str, pv_str)

    -- Column-friendly generated fields
    local col_msg = subtree:add(f_col_msg_type, tvb(0, 1), pt_str)
    col_msg:set_generated()
    col_msg:set_hidden()
    local col_rt = subtree:add(f_col_route, tvb(0, 1), rt_str)
    col_rt:set_generated()
    col_rt:set_hidden()
    local col_hops = subtree:add(f_col_hops, tvb(0, 1), hash_count)
    col_hops:set_generated()
    col_hops:set_hidden()

    if remaining <= 0 then return end

    -- Dispatch to payload-specific dissector
    if payload_type == 4 then
        dissect_advert(tvb, offset, subtree, pinfo, remaining)
    elseif payload_type == 3 then
        dissect_ack(tvb, offset, subtree, remaining)
    elseif payload_type == 5 then
        dissect_grp(tvb, offset, subtree, pinfo, remaining, "GRP_TXT")
    elseif payload_type == 6 then
        dissect_grp(tvb, offset, subtree, pinfo, remaining, "GRP_DATA")
    elseif payload_type == 7 then
        dissect_anon_req(tvb, offset, subtree, pinfo, remaining)
    elseif payload_type == 11 then
        dissect_control(tvb, offset, subtree, pinfo, remaining)
    elseif payload_type == 15 then
        local raw_tree = subtree:add(meshcore, tvb(offset, remaining), "Payload [RAW_CUSTOM]")
        raw_tree:add(f_raw_data, tvb(offset, remaining))
    -- PATH (8), TRACE (9), and MULTIPART (10) share the same encrypted envelope
    -- format as REQ/RESPONSE/TXT_MSG: dest_hash + src_hash + cipher_mac + ciphertext.
    elseif payload_type == 0 or payload_type == 1 or payload_type == 2 or
           payload_type == 8 or payload_type == 9 or payload_type == 10 then
        dissect_encrypted(tvb, offset, subtree, pinfo, remaining, pt_str)
    else
        subtree:add(f_payload, tvb(offset, remaining))
    end
end

----------------------------------------
-- Radio metadata dissector (DLT_USER0)
-- Parses variable-length radio header (radiotap-style), then hands off to meshcore.
-- Header layout:
--   byte  0:   version (0)
--   byte  1:   pad (0)
--   bytes 2-3: header length (uint16 LE) — total size including these 4 bytes
--   bytes 4-5: SNR as int16 LE (value * 100)
--   (future fields may follow; dissector skips to offset header_length)
----------------------------------------

local RADIO_HEADER_MIN = 4  -- minimum: version + pad + length

function meshcore_radio.dissector(tvb, pinfo, tree)
    local buf_len = tvb:len()
    if buf_len < RADIO_HEADER_MIN then return end

    local hdr_len = tvb(2, 2):le_uint()
    if hdr_len < RADIO_HEADER_MIN or hdr_len > buf_len then return end

    local radio_tree = tree:add(meshcore_radio, tvb(0, hdr_len), "MeshCore Radio")
    radio_tree:add_le(f_radio_version, tvb(0, 1))
    radio_tree:add_le(f_radio_pad, tvb(1, 1))
    radio_tree:add_le(f_radio_length, tvb(2, 2))

    -- SNR field (present when header length >= 6)
    if hdr_len >= 6 then
        local snr_raw = tvb(4, 2):le_int()
        radio_tree:add_le(f_radio_snr_raw, tvb(4, 2)):append_text(" (" .. format_snr(snr_raw) .. ")")
        local snr_item = radio_tree:add(f_radio_snr, tvb(4, 2), snr_raw / 100.0)
        snr_item:set_generated()
    end

    -- Show actual payload length (frame length minus header)
    local payload_len = buf_len - hdr_len
    local len_item = radio_tree:add(f_radio_payload_len, tvb(0, hdr_len), payload_len)
    len_item:set_generated()
    len_item:append_text(" bytes (frame includes " .. hdr_len .. "-byte radio header)")

    -- Hand remaining bytes to the MeshCore protocol dissector
    if hdr_len < buf_len then
        meshcore.dissector:call(tvb(hdr_len):tvb(), pinfo, tree)
    end
end

----------------------------------------
-- Register radio layer on DLT_USER0
-- DLT_USER0 is a temporary choice; it will be replaced with an officially
-- assigned LINKTYPE once one is registered with tcpdump.org.
----------------------------------------

local wtap_encap_table = DissectorTable.get("wtap_encap")
wtap_encap_table:add(wtap.USER0, meshcore_radio)
