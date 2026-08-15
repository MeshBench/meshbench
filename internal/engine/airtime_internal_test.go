// classifyAirtime tested directly, in-package: a synthetic ledger of frames
// with known byte layouts, rather than a full simulated run, because the
// thing under test is the split's arithmetic, not the radio physics that
// decide whether a frame arrives at all.
package engine

import "testing"

// textFrame builds a minimal, well-formed MeshCore flood frame carrying the
// payload type given, zero hops, and payloadLen bytes of payload - just
// enough for Dissect to find the same boundary a real frame would.
func textFrame(payloadType uint8, payloadLen int) []byte {
	header := byte(0x01) | (payloadType << 2) // flood route, no transport
	frame := []byte{header, 0x00}             // path byte: zero hops
	frame = append(frame, make([]byte, payloadLen)...)
	return frame
}

func TestAirtimeClassesSumToTheTotal(t *testing.T) {
	cases := []struct {
		name      string
		frame     []byte
		anyUnique bool
	}{
		{"payload, reached someone new", textFrame(0x02, 20), true},
		{"payload, everyone already had it", textFrame(0x02, 20), false},
		{"advert", textFrame(0x04, 20), true},
		{"ack", textFrame(0x03, 0), false},
		{"control", textFrame(0x0B, 5), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := &Engine{}
			n := &Node{}
			tr := transmission{frame: c.frame, startMs: 0, endMs: 100}
			e.classifyAirtime(n, tr, c.anyUnique)
			sum := n.AirtimePayloadMs + n.AirtimeOverheadMs + n.AirtimeRedundantMs
			if sum != 100 {
				t.Fatalf("classes sum to %v, want 100", sum)
			}
			// AirtimeMs is charged by this same call, not a separate one - the
			// two cannot drift apart because there is only one place either
			// of them is written.
			if n.AirtimeMs != sum {
				t.Fatalf("AirtimeMs=%v does not match the classes' own sum %v", n.AirtimeMs, sum)
			}
		})
	}
}

// An advert, ack or control frame carries no application payload to be
// useful or redundant about, so its whole airtime is overhead - regardless
// of who heard it.
func TestOverheadPayloadTypesAreWhollyOverhead(t *testing.T) {
	for _, pt := range []uint8{0x03, 0x04, 0x0B} {
		e := &Engine{}
		n := &Node{}
		tr := transmission{frame: textFrame(pt, 20), startMs: 0, endMs: 100}
		e.classifyAirtime(n, tr, true)
		if n.AirtimeOverheadMs != 100 {
			t.Errorf("payload type 0x%02X: overhead=%v, want the whole 100", pt, n.AirtimeOverheadMs)
		}
		if n.AirtimePayloadMs != 0 || n.AirtimeRedundantMs != 0 {
			t.Errorf("payload type 0x%02X: payload=%v redundant=%v, want both zero",
				pt, n.AirtimePayloadMs, n.AirtimeRedundantMs)
		}
	}
}

// A data-carrying frame's path-and-header bytes are overhead no matter what
// its payload does; the payload's own share goes to "payload" only when it
// reached somewhere new.
func TestDataFramesSplitPayloadFromOverhead(t *testing.T) {
	frame := textFrame(0x02, 20) // 2 header/path bytes + 20 payload bytes
	e := &Engine{}

	fresh := &Node{}
	e.classifyAirtime(fresh, transmission{frame: frame, startMs: 0, endMs: 220}, true)
	if fresh.AirtimePayloadMs <= 0 {
		t.Error("a unique delivery reported no payload airtime")
	}
	if fresh.AirtimeRedundantMs != 0 {
		t.Error("a unique delivery reported redundant airtime")
	}
	wantOverhead := 220 * 2.0 / 22.0
	if diff := fresh.AirtimeOverheadMs - wantOverhead; diff > 0.01 || diff < -0.01 {
		t.Errorf("overhead share: got %v want %v (2 of 22 bytes)", fresh.AirtimeOverheadMs, wantOverhead)
	}

	stale := &Node{}
	e.classifyAirtime(stale, transmission{frame: frame, startMs: 0, endMs: 220}, false)
	if stale.AirtimeRedundantMs <= 0 {
		t.Error("a duplicate-only delivery reported no redundant airtime")
	}
	if stale.AirtimePayloadMs != 0 {
		t.Error("a duplicate-only delivery reported payload airtime")
	}
	// Overhead is the same share either way: the path bytes cost the same
	// airtime whether or not the payload behind them turned out to be new.
	if fresh.AirtimeOverheadMs != stale.AirtimeOverheadMs {
		t.Errorf("overhead share moved with the outcome: fresh=%v stale=%v",
			fresh.AirtimeOverheadMs, stale.AirtimeOverheadMs)
	}
}

// A zero-length transmission (should not occur, but a defensive check) does
// not divide by zero or otherwise panic.
func TestClassifyAirtimeIgnoresAZeroLengthTransmission(t *testing.T) {
	e := &Engine{}
	n := &Node{}
	e.classifyAirtime(n, transmission{frame: nil, startMs: 0, endMs: 100}, true)
	if n.AirtimePayloadMs != 0 || n.AirtimeOverheadMs != 0 || n.AirtimeRedundantMs != 0 {
		t.Errorf("an empty frame charged airtime somewhere: %+v", n)
	}
}
