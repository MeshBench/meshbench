package capture_test

import (
	"strings"
	"testing"

	"github.com/A13xB0/meshcoresim/internal/capture"
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
	d := capture.Dissect(advertFrame())
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
	d := capture.Dissect([]byte{0x01 | (0x04 << 2), 0x08, 0xAA}) // claims 8 path bytes, has 1
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
	d := capture.Dissect(f)
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
	out, err := capture.RewritePath(frame, []byte{0x42, 0x42})
	if err != nil {
		t.Fatal(err)
	}
	d := capture.Dissect(out)
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
