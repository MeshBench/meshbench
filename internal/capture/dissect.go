package capture

import (
	"encoding/binary"
	"fmt"
)

// Dissection is a MeshCore frame taken apart.
//
// One dissector, shared by the workbench's inspector and the Wireshark Lua
// plugin's field set, because two dissectors of the same format eventually
// disagree and the disagreement is discovered in an argument about a capture.
type Dissection struct {
	// Header is the first byte, split.
	RouteType    uint8
	RouteName    string
	PayloadType  uint8
	PayloadName  string
	Version      uint8
	HasTransport bool

	// TransportCodes are present on the transport route types.
	TransportCodes []uint16

	// PathHashes are the node hashes a flood packet accumulates - the route
	// it actually took, and its length. Each hash is PathHashSize bytes:
	// MeshCore made that variable and encodes it in the same byte as the hop
	// count, so a path is not a byte count.
	PathHashes []byte

	// PathHashSize is how many bytes each hop's hash occupies, 1 to 4.
	PathHashSize int

	// Payload is what follows the path, and PayloadFields is whatever structure
	// the payload type lets us claim without decrypting.
	Payload       []byte
	PayloadFields []Field

	// Truncated marks a frame shorter than its own header implies. Reported
	// rather than guessed at: a frame this dissector cannot parse is evidence
	// about the frame, not a reason to invent fields.
	Truncated bool
	Problem   string
}

// Field is one named value in a dissection.
type Field struct {
	Name  string
	Value string
	// Raw is the bytes this field came from, so the hex view can highlight.
	Offset, Length int
}

// Route and payload names, from MeshCore's Packet.h. Values are wire format.
var routeNames = map[uint8]string{
	0x00: "transport flood",
	0x01: "flood",
	0x02: "direct",
	0x03: "transport direct",
}

// Payload type codes, worth testing against by name rather than as a magic
// number at each call site. Values are wire format, from MeshCore's
// Packet.h.
const (
	PayloadRequest       = 0x00
	PayloadResponse      = 0x01
	PayloadTxtMsg        = 0x02
	PayloadAck           = 0x03
	PayloadAdvert        = 0x04
	PayloadGroupText     = 0x05
	PayloadGroupDatagram = 0x06
	PayloadAnonRequest   = 0x07
	PayloadPath          = 0x08
	PayloadTrace         = 0x09
	PayloadMultipart     = 0x0A
	PayloadControl       = 0x0B
	PayloadRawCustom     = 0x0F
)

var payloadNames = map[uint8]string{
	PayloadRequest:       "request",
	PayloadResponse:      "response",
	PayloadTxtMsg:        "text message",
	PayloadAck:           "ack",
	PayloadAdvert:        "advert",
	PayloadGroupText:     "group text",
	PayloadGroupDatagram: "group datagram",
	PayloadAnonRequest:   "anonymous request",
	PayloadPath:          "returned path",
	PayloadTrace:         "trace",
	PayloadMultipart:     "multipart",
	PayloadControl:       "control",
	PayloadRawCustom:     "raw custom",
}

// IsOverheadPayload reports whether a payload type never carries an
// application message of its own - an advert, an ack, or a control frame.
// Every flood frame also carries the path bytes it has accumulated as
// overhead regardless of payload type; this only answers for the payload
// itself.
func IsOverheadPayload(t uint8) bool {
	switch t {
	case PayloadAck, PayloadAdvert, PayloadControl:
		return true
	default:
		return false
	}
}

// PayloadTypeName is the human name for a payload type, for filters and tables.
func PayloadTypeName(t uint8) string {
	if n, ok := payloadNames[t]; ok {
		return n
	}
	return fmt.Sprintf("unknown (0x%02X)", t)
}

// RouteTypeName is the human name for a route type.
func RouteTypeName(t uint8) string {
	if n, ok := routeNames[t]; ok {
		return n
	}
	return fmt.Sprintf("unknown (%d)", t)
}

// Dissect takes a frame apart as far as it can without keys.
//
// Deliberately stops at the encrypted boundary. Everything before it — routing,
// hop path, type — is what decides how a packet moves through a mesh, and it is
// all in clear; the ciphertext after it is somebody's message and not ours to
// speculate about.
func Dissect(frame []byte) Dissection {
	var d Dissection
	if len(frame) < 1 {
		d.Truncated, d.Problem = true, "empty frame"
		return d
	}

	h := frame[0]
	d.RouteType = h & 0x03
	d.PayloadType = (h >> 2) & 0x0F
	d.Version = (h >> 6) & 0x03
	d.RouteName = RouteTypeName(d.RouteType)
	d.PayloadName = PayloadTypeName(d.PayloadType)
	d.HasTransport = d.RouteType == 0x00 || d.RouteType == 0x03

	i := 1
	if d.HasTransport {
		// Two 16-bit transport codes, little-endian as MeshCore writes them.
		if len(frame) < i+4 {
			d.Truncated, d.Problem = true, "transport route with no transport codes"
			return d
		}
		d.TransportCodes = []uint16{
			binary.LittleEndian.Uint16(frame[i : i+2]),
			binary.LittleEndian.Uint16(frame[i+2 : i+4]),
		}
		i += 4
	}

	if len(frame) < i+1 {
		d.Truncated, d.Problem = true, "no path length byte"
		return d
	}
	// One byte, two fields. MeshCore packs the hash *size* into the top two
	// bits and the hop *count* into the low six (Packet.h:
	// getPathHashSize = (path_len >> 6) + 1, getPathHashCount = path_len & 63).
	//
	// Reading it as a plain byte count is why a live ScotMesh frame with
	// three-byte hashes and no hops read as "path claims 128 bytes": the
	// frame was declared truncated and dropped, and with it every region its
	// relays proved they carry. Regions matched only on the subset of the
	// network still using one-byte hashes.
	d.PathHashSize = int(frame[i]>>6) + 1
	hops := int(frame[i] & 63)
	pathLen := hops * d.PathHashSize
	i++
	if len(frame) < i+pathLen {
		d.Truncated, d.Problem = true, fmt.Sprintf(
			"path claims %d hops of %d bytes, %d present",
			hops, d.PathHashSize, len(frame)-i)
		return d
	}
	d.PathHashes = frame[i : i+pathLen]
	i += pathLen

	d.Payload = frame[i:]
	d.PayloadFields = dissectPayload(d.PayloadType, d.Payload, i)
	return d
}

// dissectPayload names the leading fields each payload type puts in clear.
func dissectPayload(t uint8, p []byte, base int) []Field {
	var out []Field
	add := func(name, value string, off, length int) {
		out = append(out, Field{Name: name, Value: value, Offset: base + off, Length: length})
	}
	switch t {
	case 0x04: // advert: public key, timestamp, signature, then app data
		if len(p) >= 32 {
			add("public key", fmt.Sprintf("%X", p[:32]), 0, 32)
		}
		if len(p) >= 36 {
			add("timestamp", fmt.Sprint(binary.LittleEndian.Uint32(p[32:36])), 32, 4)
		}
		if len(p) >= 100 {
			add("signature", fmt.Sprintf("%X…", p[36:44]), 36, 64)
		}
	case 0x05, 0x06: // group text / datagram: channel hash, MAC, ciphertext
		if len(p) >= 1 {
			add("channel hash", fmt.Sprintf("%02X", p[0]), 0, 1)
		}
		if len(p) >= 3 {
			add("MAC", fmt.Sprintf("%02X%02X", p[1], p[2]), 1, 2)
		}
		if len(p) > 3 {
			add("ciphertext", fmt.Sprintf("%d bytes", len(p)-3), 3, len(p)-3)
		}
	case 0x00, 0x01, 0x02, 0x08: // dest hash, src hash, MAC, ciphertext
		if len(p) >= 1 {
			add("destination hash", fmt.Sprintf("%02X", p[0]), 0, 1)
		}
		if len(p) >= 2 {
			add("source hash", fmt.Sprintf("%02X", p[1]), 1, 1)
		}
		if len(p) >= 4 {
			add("MAC", fmt.Sprintf("%02X%02X", p[2], p[3]), 2, 2)
		}
		if len(p) > 4 {
			add("ciphertext", fmt.Sprintf("%d bytes", len(p)-4), 4, len(p)-4)
		}
	case 0x03: // ack: a 4-byte checksum, in clear
		if len(p) >= 4 {
			add("ack checksum", fmt.Sprintf("%08X", binary.LittleEndian.Uint32(p[:4])), 0, 4)
		}
	case 0x09: // trace: tag, auth, flags, then SNR per hop
		if len(p) >= 4 {
			add("trace tag", fmt.Sprintf("%08X", binary.LittleEndian.Uint32(p[:4])), 0, 4)
		}
		if len(p) >= 9 {
			add("flags", fmt.Sprintf("%02X", p[8]), 8, 1)
		}
	}
	return out
}

// HopCount is how many nodes have relayed this frame — the number an operator
// reads first.
// HopCount is how many nodes carried this packet - hops, not bytes. With
// multi-byte hashes those stopped being the same number.
func (d Dissection) HopCount() int {
	if d.PathHashSize <= 1 {
		return len(d.PathHashes)
	}
	return len(d.PathHashes) / d.PathHashSize
}

// Hop is one hop's hash, whatever width the packet uses.
func (d Dissection) Hop(i int) []byte {
	sz := d.PathHashSize
	if sz <= 0 {
		sz = 1
	}
	if i < 0 || (i+1)*sz > len(d.PathHashes) {
		return nil
	}
	return d.PathHashes[i*sz : (i+1)*sz]
}

// Summary is one line for a table row.
func (d Dissection) Summary() string {
	if d.Truncated {
		return "malformed: " + d.Problem
	}
	return fmt.Sprintf("%s, %s, %d hop(s), %d byte payload",
		d.PayloadName, d.RouteName, d.HopCount(), len(d.Payload))
}

// RewritePath returns the frame with its path replaced.
//
// Live replay needs this: a recorded frame's path bytes are the *real*
// network's node hashes, which identify nothing in a simulation keyed
// differently. The injector rewrites the path to hashes that mean something
// here — and pads it to a minimum, because a short path is a large remaining
// flood budget, and replaying a busy mesh at full budget is a collision storm
// the real network never had.
func RewritePath(frame, path []byte) ([]byte, error) {
	d := Dissect(frame)
	if d.Truncated {
		return nil, fmt.Errorf("capture: cannot rewrite a truncated frame: %s", d.Problem)
	}
	if len(path) > 255 {
		return nil, fmt.Errorf("capture: path of %d bytes does not fit its length byte", len(path))
	}
	head := 1
	if d.HasTransport {
		head += 4
	}
	out := make([]byte, 0, head+1+len(path)+len(d.Payload))
	out = append(out, frame[:head]...)
	out = append(out, byte(len(path)))
	out = append(out, path...)
	out = append(out, d.Payload...)
	return out, nil
}
