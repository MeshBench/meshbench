package engine

import (
	"math"
	"testing"
)

// A native node's identity is keyed on its name, not its index, so node 0 of
// one fixture is not node 0 of another. It used to be seed + index*K, and two
// fixtures opened into one profile saw each other's nodes as themselves.
func TestNativeIdentitySeedIsKeyedOnTheName(t *testing.T) {
	const seed = 9001

	// Distinct names under one run seed give distinct seeds.
	a := nativeIdentitySeed(seed, "Abernethy Repeater")
	b := nativeIdentitySeed(seed, "AngusOutlaw1")
	if a == b {
		t.Fatal("two names produced one seed")
	}

	// The same name under one run seed is reproducible.
	if nativeIdentitySeed(seed, "Abernethy Repeater") != a {
		t.Error("the same node did not reproduce its seed")
	}

	// The same name under a different run seed differs, so a reseeded run is a
	// different mesh.
	if nativeIdentitySeed(seed+1, "Abernethy Repeater") == a {
		t.Error("a different run seed did not move the node's seed")
	}

	// Never zero: the firmware's own default is zero and would read as unseeded.
	if nativeIdentitySeed(0, "") == 0 {
		t.Error("the empty case produced a zero seed")
	}

	// The bridge keeps only the low 32 bits (the firmware's --seed parser reads
	// a uint32), so distinctness has to survive the truncation.
	lowA := uint32(a)
	lowB := uint32(b)
	if lowA == lowB {
		t.Errorf("two names collide in their low 32 bits: %08x", lowA)
	}
	// And no name maps to the run seed's own low word by construction.
	if uint64(lowA) == (seed & math.MaxUint32) {
		t.Error("a node's low word equals the bare run seed")
	}
}
