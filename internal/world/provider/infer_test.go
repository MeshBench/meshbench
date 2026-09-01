package provider_test

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/world/provider"
)

func frame(route, payload byte, path ...byte) []byte {
	f := []byte{route | (payload << 2)}
	if route == 0x00 || route == 0x03 {
		f = append(f, 0x34, 0x12, 0x78, 0x56) // transport codes
	}
	f = append(f, byte(len(path)))
	return append(f, path...)
}

// The two things observed traffic really proves, and the one it cannot.
func TestInferenceFromObservedPackets(t *testing.T) {
	packets := []provider.PacketRecord{
		// alpha originated a scoped advert: no path yet, transport route.
		{Raw: frame(0x00, 0x04), Sender: "alpha", Origin: "alpha"},
		// bravo relayed it — the path has a hop on it now.
		{Raw: frame(0x00, 0x04, 0x9F), Sender: "bravo"},
		// charlie only ever relays unscoped floods.
		{Raw: frame(0x01, 0x05, 0x3C), Sender: "charlie"},
	}
	got := provider.InferFromPackets(packets, nil)

	if a := got["alpha"]; a == nil || !a.ScopedOrigin {
		t.Error("alpha originated scoped traffic, so it has a default scope set")
	}
	if b := got["bravo"]; b == nil || !b.ScopedRelay {
		t.Error("bravo relayed scoped traffic, so it holds a region that admits it")
	}
	if c := got["charlie"]; c == nil || c.ScopedRelay {
		t.Error("charlie has only ever relayed unscoped traffic")
	}
	if c := got["charlie"]; c == nil || !c.UnscopedRelay {
		t.Error("charlie's unscoped relay was not recorded")
	}

	// The honest limit: a region's *name* is not recoverable, because the
	// transport code is hashed with the packet. Nothing may claim otherwise.
	if b := got["bravo"]; b != nil && len(b.PayloadTypes) == 0 {
		t.Error("payload types should be recorded even where the region name cannot be")
	}
}

// Two candidate names that collide on the same transport code must still
// give alpha a single, repeatable default scope. The advert loop in
// inferFromOnePacket overwrites DefaultScope on every match it sees, so
// whichever name Match returns last decides - and since Match orders its
// matches alphabetically, that is always the alphabetically later of the
// two colliding names, not whichever the candidate map happened to hash to
// first.
func TestInferDefaultScopeCollisionIsDeterministic(t *testing.T) {
	payload := []byte{0x11, 0x22, 0x33, 0x44}
	adv := scopedFrame(collidingRegionA, 0x04, payload)
	m := provider.NewNamedRegions([]string{collidingRegionB, collidingRegionA})
	packets := []provider.PacketRecord{
		{Raw: adv, Sender: "alpha", Origin: "alpha"},
	}

	for i := 0; i < 5; i++ {
		got := provider.InferFromPackets(packets, m)
		a := got["alpha"]
		if a == nil {
			t.Fatalf("run %d: alpha not inferred", i)
		}
		if a.DefaultScope != collidingRegionB {
			t.Fatalf("run %d: default scope = %q, want %q (alphabetically later of the colliding pair)",
				i, a.DefaultScope, collidingRegionB)
		}
	}
}

// A node nobody heard is not a node with no regions — it is a node with no
// evidence, and the difference matters when the conclusion drives a change.
func TestUnseenNodesAreNotAssumedEmpty(t *testing.T) {
	got := provider.InferFromPackets(nil, nil)
	if len(got) != 0 {
		t.Errorf("inferred %d nodes from no traffic", len(got))
	}
	var none provider.Inferred
	if none.Summary() != "never seen" {
		t.Errorf("an unseen node summarised as %q", none.Summary())
	}
}
