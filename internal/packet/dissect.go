package packet

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

	// PathFields names the path area entry by entry - relay hashes, or, for a
	// trace, the SNR each hop measured.
	PathFields []Field

	// Spans is the frame's shape: which bytes are header, transport codes,
	// path and payload.
	Spans []Span

	// Truncated marks a frame shorter than its own header implies. Reported
	// rather than guessed at: a frame this dissector cannot parse is evidence
	// about the frame, not a reason to invent fields.
	Truncated bool
	Problem   string
}

// Field is one named value in a dissection.
type Field struct {
	Name string
	// Value is what is on the wire, kept as the evidence.
	Value string
	// Decoded is that same value in the form a person reads - a timestamp as
	// a date, a flags byte as the things it switches on, a fixed-point
	// coordinate as degrees. Empty when the raw value is already readable.
	//
	// Both, never one: the decoded reading is the answer, and the raw value
	// is what lets somebody check it against the hex beside it.
	Decoded string
	// Description says what the field is for, in a few words.
	Description string
	// Raw is the bytes this field came from, so the hex view can highlight.
	Offset, Length int
}

// Span is one structural region of a frame - the header, the transport codes,
// the path, the payload. Enough to show the shape of a frame before any of
// its detail, and to say how many bytes each part cost.
type Span struct {
	Name           string
	Offset, Length int
	// Detail is what to say about this span beside its size.
	Detail string
}

// Route and payload names, from MeshCore's Packet.h. Values are wire format.
var routeNames = map[uint8]string{
	0x00: "transport flood",
	0x01: "flood",
	0x02: "direct",
	0x03: "transport direct",
}

var payloadNames = map[uint8]string{
	0x00: "request",
	0x01: "response",
	0x02: "text message",
	0x03: "ack",
	0x04: "advert",
	0x05: "group text",
	0x06: "group datagram",
	0x07: "anonymous request",
	0x08: "returned path",
	0x09: "trace",
	0x0A: "multipart",
	0x0B: "control",
	0x0F: "raw custom",
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
	d.Spans = append(d.Spans, Span{
		Name: "header", Offset: 0, Length: 1,
		Detail: fmt.Sprintf("%s, %s, version %d", d.RouteName, d.PayloadName, d.Version),
	})

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
		// Four bytes, and the path length byte after them is not one of them.
		// Only code [0] is ever matched against a region (RegionMap::findMatch);
		// [1] is carried and reserved. Naming them as anything else - a next
		// hop, an address - would be a confident invention.
		d.Spans = append(d.Spans, Span{
			Name: "transport codes", Offset: i, Length: 4,
			Detail: scopeCodeDetail(d.TransportCodes),
		})
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
	d.Spans = append(d.Spans, Span{
		Name: "path length", Offset: i, Length: 1,
		Detail: fmt.Sprintf("%d hops of %d bytes", hops, d.PathHashSize),
	})
	i++
	if len(frame) < i+pathLen {
		d.Truncated, d.Problem = true, fmt.Sprintf(
			"path claims %d hops of %d bytes, %d present",
			hops, d.PathHashSize, len(frame)-i)
		return d
	}
	d.PathHashes = frame[i : i+pathLen]
	if pathLen > 0 {
		d.Spans = append(d.Spans, Span{
			Name: "path", Offset: i, Length: pathLen, Detail: pathSpanDetail(d),
		})
	}
	d.PathFields = pathEntries(d, i)
	i += pathLen

	d.Payload = frame[i:]
	d.PayloadFields = dissectPayload(d.PayloadType, d.Version, d.Payload, i)
	if len(d.Payload) > 0 {
		d.Spans = append(d.Spans, Span{
			Name: "payload — " + d.PayloadName, Offset: i, Length: len(d.Payload),
			Detail: payloadSpanDetail(d.PayloadType),
		})
	}
	return d
}

// scopeCodeDetail says what the two transport codes are without overstating
// what can be known from them. The region key is not in the packet, so a code
// can only ever be checked against a candidate name - never turned back into
// one - and that is the caller's job, not the dissector's.
func scopeCodeDetail(codes []uint16) string {
	if len(codes) < 2 {
		return ""
	}
	if codes[0] == 0 && codes[1] == 0 {
		return "0000 0000 — addressed to no region"
	}
	return fmt.Sprintf("scope code %04X, reserved %04X", codes[0], codes[1])
}

// pathSpanDetail names the path area for what it holds, which is not always
// hashes: a trace collects one signed SNR byte per hop here instead.
func pathSpanDetail(d Dissection) string {
	if d.PayloadType == 0x09 {
		return fmt.Sprintf("%d SNR readings collected by the trace", len(d.PathHashes))
	}
	return fmt.Sprintf("%d relays, %d bytes each", d.HopCount(), d.PathHashSize)
}

// payloadSpanDetail says up front whether there is anything to read, so
// nobody goes hunting through a payload for a message body that was only ever
// going to be ciphertext.
func payloadSpanDetail(t uint8) string {
	switch t {
	case 0x03, 0x04, 0x09:
		return "in clear"
	case 0x0B, 0x0A:
		return "partly in clear"
	case 0x0F:
		return "application-defined"
	}
	return "prefix in clear, body encrypted"
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
