package emulated

import "testing"

// The seed has one job beyond being reproducible: it must differ.
//
// Every emulated nRF52 board reported the same public key because the radio
// handed the firmware the same "random" bytes, and the firmware derives its
// identity from them. Seeding by node name alone would have fixed the
// within-a-run half and left a node with the same identity in every scenario
// for ever, so the run's own seed is mixed in too.
func TestNoiseSeedsDiffer(t *testing.T) {
	const runA, runB = uint64(4417), uint64(9001)

	if noiseSeedFor(runA, "bc-under-test") == noiseSeedFor(runA, "bc-sender") {
		t.Error("two nodes in one run share a seed, so they will share an identity")
	}
	if noiseSeedFor(runA, "bc-under-test") == noiseSeedFor(runB, "bc-under-test") {
		t.Error("one node in two differently seeded runs shares a seed")
	}
	// Held in variables rather than compared inline: the point is that the
	// answer does not drift between calls, and a compiler is entitled to notice
	// that two identical calls to a pure function cannot.
	first := noiseSeedFor(runA, "n")
	second := noiseSeedFor(runA, "n")
	if first != second {
		t.Error("the same run and node gave two answers; reproducibility is a rule here")
	}
	if noiseSeedFor(0, "") == 0 {
		t.Error("a zero seed is the broken case: both Arduino cores ignore it")
	}
}
