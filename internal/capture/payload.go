package capture

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Reading what each payload type puts in clear.
//
// Split from the frame walk in dissect.go because they answer different
// questions: that file knows how a MeshCore frame is shaped, this one knows
// what thirteen different payloads mean once you are inside one. Everything
// here is read from the firmware that writes it (repeater-v1.17.0) rather
// than from documentation about it - Packet.h names the types and, in its own
// comments, the in-clear prefix each one carries.
//
// The rule this file exists to keep: name what is in clear, say plainly what
// is encrypted, and never render ciphertext as though it meant something.

// Payload versions, from the header's top two bits. Only the first is defined
// in v1.17.0; the other three are reserved. An undefined version is reported
// as undefined rather than parsed with version 1's field widths, which would
// put every offset after the first hash silently in the wrong place.
const (
	payloadVer1 = 0
)

// Version 1's widths: 1-byte src/dest hashes, 2-byte MAC.
const (
	ver1HashSize = 1
	ver1MACSize  = 2
)

// Advert app_data flags, from AdvertDataHelpers.h.
const (
	advTypeMask   = 0x0F
	advLatLonMask = 0x10
	advFeat1Mask  = 0x20
	advFeat2Mask  = 0x40
	advNameMask   = 0x80
)

// advertNodeTypes is the low nibble of an advert's flags byte.
var advertNodeTypes = map[uint8]string{
	0: "none", 1: "chat", 2: "repeater", 3: "room server", 4: "sensor",
}

// rd walks a payload, naming each span it reads and keeping the offsets
// straight so the hex view can highlight exactly the bytes a field came from.
type rd struct {
	p []byte
	// base is where p[0] sits in the whole frame, so an offset is a frame
	// offset and not a payload one - the hex dump numbers the frame.
	base int
	at   int
	out  []Field
}

func (r *rd) left() int { return len(r.p) - r.at }

// field names the next n bytes and steps over them, reporting false when the
// payload is too short to hold them. A caller that ignores that would emit a
// field describing bytes that are not there.
func (r *rd) field(n int, name, desc string, render func([]byte) (string, string)) bool {
	if n < 0 || r.left() < n {
		return false
	}
	b := r.p[r.at : r.at+n]
	value, decoded := render(b)
	r.out = append(r.out, Field{
		Name: name, Value: value, Decoded: decoded, Description: desc,
		Offset: r.base + r.at, Length: n,
	})
	r.at += n
	return true
}

// rest names everything left, if anything is.
func (r *rd) rest(name, desc string, render func([]byte) (string, string)) {
	if r.left() <= 0 {
		return
	}
	r.field(r.left(), name, desc, render)
}

// ciphertext closes a payload that we deliberately stop at. The count is the
// useful part: it says how much is there without pretending to know what.
func (r *rd) ciphertext(what string) {
	if r.left() <= 0 {
		return
	}
	n := r.left()
	r.field(n, "encrypted", what, func([]byte) (string, string) {
		return fmt.Sprintf("%d bytes", n), "not decryptable - the key is not in the packet"
	})
}

// --- renderers -------------------------------------------------------------

func asHex(b []byte) (string, string) { return fmt.Sprintf("%X", b), "" }

// asHexAbbrev keeps a long key or signature to a readable width; the copy
// button hands over the whole thing, so nothing is lost by not printing it.
func asHexAbbrev(b []byte) (string, string) {
	s := fmt.Sprintf("%X", b)
	if len(s) <= 24 {
		return s, ""
	}
	return s[:16] + "…" + s[len(s)-8:], fmt.Sprintf("%d bytes", len(b))
}

func asByteHex(b []byte) (string, string) { return fmt.Sprintf("0x%02X", b[0]), "" }

func asU16LE(b []byte) (string, string) {
	return fmt.Sprintf("0x%04X", binary.LittleEndian.Uint16(b)), ""
}

func asU32HexLE(b []byte) (string, string) {
	return fmt.Sprintf("%08X", binary.LittleEndian.Uint32(b)), ""
}

// asEpochLE reads MeshCore's timestamps: uint32 seconds, little-endian, as
// the RTC hands them to createAdvert.
func asEpochLE(b []byte) (string, string) {
	v := binary.LittleEndian.Uint32(b)
	return fmt.Sprint(v), time.Unix(int64(v), 0).UTC().Format("2006-01-02 15:04:05Z")
}

// asLatLon is MeshCore's fixed-point degrees: int32, degrees times 1e6.
func asLatLon(b []byte) (string, string) {
	v := int32(binary.LittleEndian.Uint32(b))
	return fmt.Sprint(v), fmt.Sprintf("%.6f°", float64(v)/1e6)
}

// dissectPayload names the leading fields each payload type puts in clear.
//
// version is the header's payload version, which decides how wide the src and
// dest hashes and the MAC are on the types that carry them. pathHashSize is
// only wanted by trace, whose payload carries the path it is tracing.
func dissectPayload(t uint8, version uint8, p []byte, base int) []Field {
	r := &rd{p: p, base: base}
	switch t {
	case 0x04:
		readAdvert(r)
	case 0x03:
		readAck(r)
	case 0x09:
		readTrace(r)
	case 0x07:
		readAnonRequest(r, version)
	case 0x05, 0x06:
		readGroup(r, t, version)
	case 0x00, 0x01, 0x02, 0x08:
		readAddressed(r, t, version)
	case 0x0A:
		readMultipart(r)
	case 0x0B:
		readControl(r)
	case 0x0F:
		r.rest("raw", "application-defined; MeshCore does not specify this payload", asHexAbbrev)
	}
	return r.out
}

// readAdvert reads the one payload type that is entirely in clear: an
// identity, when it said so, a signature over both, and the app data that
// carries what the node is, where it is and what it calls itself.
func readAdvert(r *rd) {
	r.field(32, "public key", "the node's identity - Ed25519", asHexAbbrev)
	r.field(4, "timestamp", "when the advert was emitted", asEpochLE)
	r.field(64, "signature", "Ed25519 over public key, timestamp and app data", asHexAbbrev)
	readAdvertAppData(r)
}

// readAdvertAppData walks the optional advert fields.
//
// A flags byte, then each field present only if its own bit is set, in a
// fixed order (AdvertDataBuilder::encodeTo). Nothing here is a fixed struct:
// skip a bit and every field after it is misread, which is why this is a walk
// and each step is conditional on the flag that announces it.
func readAdvertAppData(r *rd) {
	if r.left() < 1 {
		return
	}
	flags := r.p[r.at]
	r.field(1, "flags", "which advert fields follow", func(b []byte) (string, string) {
		return fmt.Sprintf("0x%02X", b[0]), describeAdvertFlags(b[0])
	})
	if flags&advLatLonMask != 0 {
		r.field(4, "latitude", "degrees × 1e6", asLatLon)
		r.field(4, "longitude", "degrees × 1e6", asLatLon)
	}
	if flags&advFeat1Mask != 0 {
		r.field(2, "feature 1", "reserved by MeshCore for future use", asU16LE)
	}
	if flags&advFeat2Mask != 0 {
		r.field(2, "feature 2", "reserved by MeshCore for future use", asU16LE)
	}
	if flags&advNameMask != 0 {
		r.rest("name", "UTF-8, to the end of the advert - not NUL-terminated", asAdvertName)
	}
}

// asAdvertName renders the node's own name.
//
// It runs to the end of app_data and is not NUL-terminated, and the firmware
// truncates it to a whole-rune prefix on the way out (validUtf8PrefixLength).
// A frame that arrived cut short mid-rune is reported as such rather than
// rendered with a mangled tail.
func asAdvertName(b []byte) (string, string) {
	s := strings.TrimRight(string(b), "\x00")
	if utf8.ValidString(s) {
		return s, ""
	}
	return strings.ToValidUTF8(s, "�"), "truncated mid-character"
}

// describeAdvertFlags spells out a flags byte in the terms an operator reads.
func describeAdvertFlags(f uint8) string {
	kind, ok := advertNodeTypes[f&advTypeMask]
	if !ok {
		kind = fmt.Sprintf("type %d", f&advTypeMask)
	}
	var has []string
	if f&advLatLonMask != 0 {
		has = append(has, "position")
	}
	if f&advFeat1Mask != 0 {
		has = append(has, "feature 1")
	}
	if f&advFeat2Mask != 0 {
		has = append(has, "feature 2")
	}
	if f&advNameMask != 0 {
		has = append(has, "name")
	}
	if len(has) == 0 {
		return kind
	}
	return kind + ", with " + strings.Join(has, ", ")
}

// readAck: the whole payload is a checksum, in clear.
func readAck(r *rd) {
	r.field(4, "ack checksum", "identifies the message being acknowledged", asU32HexLE)
}

// readTrace reads a path trace.
//
// The payload carries the path being traced - one hash per hop, each
// 1<<(flags&3) bytes wide. The SNR each hop measured is *not* here: it is
// appended to the packet's own path area as it travels, which is why
// pathEntries reads that area as signed SNR for this type alone.
func readTrace(r *rd) {
	if r.left() < 9 {
		return
	}
	flags := r.p[r.at+8]
	r.field(4, "trace tag", "identifies this trace across its replies", asU32HexLE)
	r.field(4, "auth code", "authorises the trace to the nodes it crosses", asU32HexLE)
	r.field(1, "flags", "low two bits are the path hash size", func(b []byte) (string, string) {
		return fmt.Sprintf("0x%02X", b[0]), fmt.Sprintf("%d-byte path hashes", 1<<(b[0]&0x03))
	})
	entry := 1 << (flags & 0x03)
	for hop := 0; r.left() >= entry; hop++ {
		r.field(entry, fmt.Sprintf("path hop %d", hop+1),
			"a node this trace is routed through", asHex)
	}
	r.rest("trailing", "left over after the whole path was read", asHex)
}

// readAnonRequest reads the type that hands over a whole public key in clear.
//
// The sender is anonymous only in the sense of not being an established
// contact; the key that identifies it is right there, which is worth naming
// because it is the one encrypted-body type whose sender can be pinned down.
func readAnonRequest(r *rd, version uint8) {
	if !ver1(r, version) {
		return
	}
	r.field(ver1HashSize, "destination hash", "first byte of the recipient's public key", asByteHex)
	r.field(32, "sender public key", "the requester's identity, in clear", asHexAbbrev)
	r.field(ver1MACSize, "MAC", "authenticates the encrypted body", asHex)
	r.ciphertext("the request body")
}

// readGroup reads the channel-addressed types.
func readGroup(r *rd, t uint8, version uint8) {
	if !ver1(r, version) {
		return
	}
	r.field(ver1HashSize, "channel hash", "which channel's key this was encrypted to", asByteHex)
	r.field(ver1MACSize, "MAC", "authenticates the encrypted body", asHex)
	if t == 0x05 {
		r.ciphertext("a timestamp and the message text")
	} else {
		r.ciphertext("a data type, a length and the data")
	}
}

// readAddressed reads the node-to-node types, which share a prefix.
func readAddressed(r *rd, t uint8, version uint8) {
	if !ver1(r, version) {
		return
	}
	r.field(ver1HashSize, "destination hash", "first byte of the recipient's public key", asByteHex)
	r.field(ver1HashSize, "source hash", "first byte of the sender's public key", asByteHex)
	r.field(ver1MACSize, "MAC", "authenticates the encrypted body", asHex)
	switch t {
	case 0x02:
		r.ciphertext("a timestamp and the message text")
	case 0x08:
		r.ciphertext("the returned path, and any extra")
	default:
		r.ciphertext("a timestamp and the request or response body")
	}
}

// readMultipart: one byte says how many more packets are coming and what type
// the set carries, so a multipart packet can be read without its siblings.
func readMultipart(r *rd) {
	r.field(1, "part header", "remaining count in the high nibble, carried type in the low", func(b []byte) (string, string) {
		return fmt.Sprintf("0x%02X", b[0]), fmt.Sprintf("%d more to come, carrying %s",
			b[0]>>4, PayloadTypeName(b[0]&0x0F))
	})
	r.rest("part data", "belongs to the carried payload type, not this one", asHexAbbrev)
}

// readControl: a control or discovery packet. Bit 7 of the first byte marks
// the zero-hop subset the mesh will accept without forwarding.
func readControl(r *rd) {
	if r.left() < 1 {
		return
	}
	r.field(1, "control byte", "bit 7 marks the zero-hop control subset", func(b []byte) (string, string) {
		if b[0]&0x80 != 0 {
			return fmt.Sprintf("0x%02X", b[0]), "zero-hop control"
		}
		return fmt.Sprintf("0x%02X", b[0]), "forwarded control"
	})
	r.rest("control data", "MeshCore does not define this further in v1.17.0", asHexAbbrev)
}

// ver1 guards the types whose field widths are version-dependent.
//
// Only PAYLOAD_VER_1 is defined; the header reserves two bits for three more.
// Parsing an unknown version with version 1's widths would produce a full set
// of confidently mislabelled fields, so it says what it does not know instead.
func ver1(r *rd, version uint8) bool {
	if version == payloadVer1 {
		return true
	}
	n := r.left()
	r.field(n, "payload", fmt.Sprintf(
		"payload version %d is not defined in MeshCore v1.17.0, so the field "+
			"widths are unknown and nothing here is parsed", version),
		func([]byte) (string, string) { return fmt.Sprintf("%d bytes", n), "not parsed" })
	return false
}

// pathEntries names the frame's path area.
//
// Hashes for every type but one. A trace collects the SNR each hop measured
// in this same area - one signed byte per hop, scaled by four
// (Mesh.cpp: path[path_len++] = (int8_t)(getSNR()*4)) - so reading it as
// hashes there would print the measurement this packet exists to carry as
// meaningless hex.
func pathEntries(d Dissection, offset int) []Field {
	if d.HopCount() == 0 {
		return nil
	}
	var out []Field
	if d.PayloadType == 0x09 {
		for i, b := range d.PathHashes {
			out = append(out, Field{
				Name:        fmt.Sprintf("hop %d SNR", i+1),
				Value:       fmt.Sprintf("%d", int8(b)),
				Decoded:     fmt.Sprintf("%+.2f dB", float64(int8(b))/4),
				Description: "measured by this hop as the trace passed",
				Offset:      offset + i, Length: 1,
			})
		}
		return out
	}
	sz := d.PathHashSize
	if sz <= 0 {
		sz = 1
	}
	for i := 0; i+sz <= len(d.PathHashes); i += sz {
		out = append(out, Field{
			Name:        fmt.Sprintf("hop %d", i/sz+1),
			Value:       fmt.Sprintf("%X", d.PathHashes[i:i+sz]),
			Description: "a node that relayed this packet",
			Offset:      offset + i, Length: sz,
		})
	}
	return out
}
