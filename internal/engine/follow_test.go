package engine_test

import (
	"testing"

	"github.com/A13xB0/meshcoresim/internal/engine"
)

// A message must keep one identity across every hop.
//
// MeshCore appends a hop hash to a flood packet's path at each relay, so the
// bytes on the air are different at every hop. Hashing the whole frame — which
// is what this used to do — gave the same message a new identity each time: it
// could not be followed across the mesh, every relay counted as a brand new
// message, and the unique-versus-redundant figure was measuring nothing.
func TestAMessageKeepsOneIdentityAcrossHops(t *testing.T) {
	payload := []byte{0xAA, 0xBB, 0xCC, 0xDD}

	// The same group text, seen at three points in its life: fresh from the
	// origin, after one relay, and after two.
	frames := [][]byte{
		append([]byte{0x01 | (0x05 << 2), 0x00}, payload...),
		append([]byte{0x01 | (0x05 << 2), 0x01, 0x9F}, payload...),
		append([]byte{0x01 | (0x05 << 2), 0x02, 0x9F, 0x3C}, payload...),
	}

	first := engine.PayloadIDForTest(frames[0])
	for i, f := range frames[1:] {
		if got := engine.PayloadIDForTest(f); got != first {
			t.Errorf("hop %d has a different identity from the origin; the path is being "+
				"hashed and the message cannot be followed", i+1)
		}
	}

	// Two genuinely different messages must not collide, or "followed" would
	// mean "confused with".
	other := append([]byte{0x01 | (0x05 << 2), 0x00}, 0xAA, 0xBB, 0xCC, 0xDE)
	if engine.PayloadIDForTest(other) == first {
		t.Error("two different payloads share an identity")
	}

	// A re-routed packet is still the same message: a node may forward a flood
	// packet as direct, and an identity that changed there would break at the
	// most interesting moment.
	rerouted := append([]byte{0x02 | (0x05 << 2), 0x01, 0x9F}, payload...)
	if engine.PayloadIDForTest(rerouted) != first {
		t.Error("changing route type changed the message's identity")
	}
}
