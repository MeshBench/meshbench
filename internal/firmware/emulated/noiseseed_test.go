package emulated

import "testing"

// here is somewhere to stand, for the cases that are not about position.
var here = LatLon{Lat: 56.70, Lon: -3.85}

// The seed has one job beyond being reproducible: it must differ.
//
// Every emulated nRF52 board reported the same public key because the radio
// handed the firmware the same "random" bytes, and the firmware derives its
// identity from them. Seeding by node name alone would have fixed the
// within-a-run half and left a node with the same identity in every scenario
// for ever, so the run's own seed is mixed in too.
func TestNoiseSeedsDiffer(t *testing.T) {
	const runA, runB = uint64(4417), uint64(9001)
	const board = "LilyGo_TDeck"

	if noiseSeedFor(runA, "bc-under-test", board, here) ==
		noiseSeedFor(runA, "bc-sender", board, here) {
		t.Error("two nodes in one run share a seed, so they will share an identity")
	}
	if noiseSeedFor(runA, "bc-under-test", board, here) ==
		noiseSeedFor(runB, "bc-under-test", board, here) {
		t.Error("one node in two differently seeded runs shares a seed")
	}
	// Held in variables rather than compared inline: the point is that the
	// answer does not drift between calls, and a compiler is entitled to notice
	// that two identical calls to a pure function cannot.
	first := noiseSeedFor(runA, "n", board, here)
	second := noiseSeedFor(runA, "n", board, here)
	if first != second {
		t.Error("the same run and node gave two answers; reproducibility is a rule here")
	}
	if noiseSeedFor(0, "", "", LatLon{}) == 0 {
		t.Error("a zero seed is the broken case: both Arduino cores ignore it")
	}
}

// Two boards of one name in one place are two nodes.
//
// This is the case that was actually wrong. The board probe calls every board
// it measures "bc-under-test" and stands it at the same coordinates, so a
// LilyGo T-Deck under QEMU and a RAK4631 under Renode came up as the same node,
// byte for byte - which reads exactly like one board's stored state leaking
// into another's, and cost a measurement before it was understood.
func TestTwoBoardsOfOneNameDiffer(t *testing.T) {
	const run = uint64(4417)
	if noiseSeedFor(run, "bc-under-test", "LilyGo_TDeck", here) ==
		noiseSeedFor(run, "bc-under-test", "RAK_4631", here) {
		t.Error("two boards of one name share a seed, so they share an identity")
	}
}

// And two nodes of one name in different places.
func TestOneNameInTwoPlacesDiffers(t *testing.T) {
	const run = uint64(4417)
	there := LatLon{Lat: 56.70, Lon: -3.90}
	if noiseSeedFor(run, "n", "b", here) == noiseSeedFor(run, "n", "b", there) {
		t.Error("one name in two places shares a seed")
	}
}

// The fields are mixed with a separator, so a node cannot be spelled two ways.
//
// Without one, ("ab", "c") and ("a", "bc") hash identically - and a board named
// for its node, or a node for its board, is exactly the sort of pair that
// happens.
func TestTheFieldsCannotRunTogether(t *testing.T) {
	const run = uint64(4417)
	if noiseSeedFor(run, "ab", "c", here) == noiseSeedFor(run, "a", "bc", here) {
		t.Error("the name and the board run together, so two different nodes hash alike")
	}
}

// Position is quantised, not taken as float bits.
//
// A seed that varied with how a coordinate was rounded on the way in would take
// reproducibility across machines with it. A microdegree is about a tenth of a
// metre, finer than any two nodes anybody places apart, and a difference below
// it is the same place.
func TestPositionIsQuantised(t *testing.T) {
	const run = uint64(4417)
	a := LatLon{Lat: 56.70, Lon: -3.85}
	b := LatLon{Lat: 56.70 + 1e-9, Lon: -3.85 - 1e-9}
	if noiseSeedFor(run, "n", "b", a) != noiseSeedFor(run, "n", "b", b) {
		t.Error("a nanodegree moved the seed, so it depends on float rounding")
	}
	if got := microDegrees(56.70); got != 56_700_000 {
		t.Errorf("microDegrees(56.70) = %d, want 56700000", got)
	}
	// Rounded away from zero on both sides, so a coordinate exactly between two
	// microdegrees does not depend on its sign.
	if got := microDegrees(-0.0000005); got != -1 {
		t.Errorf("microDegrees(-0.0000005) = %d, want -1", got)
	}
	if got := microDegrees(0.0000005); got != 1 {
		t.Errorf("microDegrees(0.0000005) = %d, want 1", got)
	}
}

// A node with no position is seeded, not skipped.
//
// An import can leave a node without one, and NaN converts to an
// implementation-defined integer rather than to anything a seed should carry.
func TestAPositionlessNodeStillSeeds(t *testing.T) {
	if got := microDegrees(nan()); got != 0 {
		t.Errorf("microDegrees(NaN) = %d, want 0", got)
	}
	if noiseSeedFor(4417, "n", "b", LatLon{Lat: nan(), Lon: nan()}) == 0 {
		t.Error("a positionless node seeded zero, which is the broken case")
	}
}

func nan() float64 { var z float64; return z / z }
