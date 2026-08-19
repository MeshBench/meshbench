package packet_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/packet"
)

// A flood advert, as MeshCore builds one: header, path length, two hops of
// path, then the advert payload.
func advertFrame() []byte {
	f := []byte{
		0x01 | (0x04 << 2), // flood route, advert payload
		0x02, 0xAB, 0xCD,   // two path hashes
	}
	pub := make([]byte, 32)
	for i := range pub {
		pub[i] = byte(i)
	}
	f = append(f, pub...)
	f = append(f, 0x10, 0x20, 0x30, 0x40) // timestamp
	return f
}

func TestDissectReadsRoutingWithoutKeys(t *testing.T) {
	d := packet.Dissect(advertFrame())
	if d.Truncated {
		t.Fatalf("a well-formed advert was called malformed: %s", d.Problem)
	}
	if d.RouteName != "flood" {
		t.Errorf("route = %q, want flood", d.RouteName)
	}
	if d.PayloadName != "advert" {
		t.Errorf("payload = %q, want advert", d.PayloadName)
	}
	// The hop count is the number an operator reads first, and it comes from
	// the path the packet accumulated rather than from anything we track.
	if d.HopCount() != 2 {
		t.Errorf("hops = %d, want 2", d.HopCount())
	}
	var sawKey bool
	for _, f := range d.PayloadFields {
		if f.Name == "public key" {
			sawKey = true
		}
	}
	if !sawKey {
		t.Error("an advert's public key is in clear and should be named")
	}
}

// A truncated frame is evidence about the frame. Inventing fields for it is how
// a dissector turns a corrupt capture into a confident wrong story.
func TestDissectReportsTruncationRatherThanGuessing(t *testing.T) {
	d := packet.Dissect([]byte{0x01 | (0x04 << 2), 0x08, 0xAA}) // claims 8 path bytes, has 1
	if !d.Truncated {
		t.Fatal("a frame claiming more path than it carries parsed cleanly")
	}
	if !strings.Contains(d.Problem, "path claims") {
		t.Errorf("the problem should say what did not add up: %q", d.Problem)
	}
	if !strings.Contains(d.Summary(), "malformed") {
		t.Errorf("summary hides the problem: %q", d.Summary())
	}
}

// Transport route types carry two codes before the path; reading them as path
// would shift every field after by four bytes.
func TestTransportCodesAreNotMistakenForPath(t *testing.T) {
	f := []byte{0x00 | (0x05 << 2), 0x34, 0x12, 0x78, 0x56, 0x01, 0x99, 0xAA, 0xBB, 0xCC}
	d := packet.Dissect(f)
	if !d.HasTransport {
		t.Fatal("a transport route type was not recognised")
	}
	if len(d.TransportCodes) != 2 || d.TransportCodes[0] != 0x1234 {
		t.Errorf("transport codes = %v, want [0x1234 0x5678]", d.TransportCodes)
	}
	if d.HopCount() != 1 {
		t.Errorf("hops = %d, want 1 — the codes were read as path", d.HopCount())
	}
}

func TestRewritePathKeepsEverythingElse(t *testing.T) {
	// flood route (0x01), advert payload type (0x04): header 0x11, no
	// transport codes, then pathLen, path, payload.
	frame := []byte{0x11, 0x02, 0xAA, 0xBB, 0xDE, 0xAD, 0xBE, 0xEF}
	out, err := packet.RewritePath(frame, []byte{0x42, 0x42})
	if err != nil {
		t.Fatal(err)
	}
	d := packet.Dissect(out)
	if len(d.PathHashes) != 2 || d.PathHashes[0] != 0x42 || d.PathHashes[1] != 0x42 {
		t.Fatalf("path = %x", d.PathHashes)
	}
	if string(d.Payload) != string([]byte{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Fatalf("payload changed: %x", d.Payload)
	}
	if out[0] != frame[0] {
		t.Fatal("header changed")
	}
}

// The live frame that exposed the variable-hash format: a #fif-scoped
// channel message from ScotMesh whose path byte is 0x80 — three-byte hashes,
// no hops yet. Read as a plain byte count that is 128 bytes of path in a
// 73-byte frame, so the dissector called it truncated and dropped it, and
// every region its relays proved went with it.
func TestVariablePathHashSize(t *testing.T) {
	raw, err := hex.DecodeString("14c47500008003bf92968994795bd4303e4df3bed4e61ce83504" +
		"eece967eb3907a133494611d225f353299022705755204f1811d04896a21945c79ff43c37bddafbb1371b616e51d49")
	if err != nil {
		t.Fatal(err)
	}
	d := packet.Dissect(raw)
	if d.Truncated {
		t.Fatalf("live frame reported truncated: %s", d.Problem)
	}
	if d.PathHashSize != 3 {
		t.Fatalf("hash size = %d, want 3 (0x80 >> 6 + 1)", d.PathHashSize)
	}
	if d.HopCount() != 0 {
		t.Fatalf("hop count = %d, want 0 (0x80 & 63)", d.HopCount())
	}
	if !d.HasTransport {
		t.Fatal("route type 0 is transport flood, so it carries transport codes")
	}
	// Payload is everything after the path, and it is what the region HMAC is
	// taken over — getting the offset wrong is silent, and produces a code
	// that matches no region at all.
	if len(d.Payload) == 0 {
		t.Fatal("no payload: the offset past the path is wrong")
	}
}

// A one-byte-hash packet with three hops still reads as three hops.
func TestSingleBytePathHashesStillWork(t *testing.T) {
	frame := []byte{0x11, 0x03, 0xAA, 0xBB, 0xCC, 0xDE, 0xAD}
	d := packet.Dissect(frame)
	if d.PathHashSize != 1 || d.HopCount() != 3 {
		t.Fatalf("size=%d hops=%d, want 1 and 3", d.PathHashSize, d.HopCount())
	}
	if string(d.Payload) != string([]byte{0xDE, 0xAD}) {
		t.Fatalf("payload = %x", d.Payload)
	}
}
