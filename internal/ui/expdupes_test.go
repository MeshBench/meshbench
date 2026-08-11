package ui

import "testing"

// Duplication is the measurement the loop-detection sweep rests on, and it was
// absent until this change: reach was a set of nodes per message, so a node that
// heard one copy and a node that heard nine counted the same. Every arm that
// differed only in how much redundant traffic it suppressed looked identical,
// which is exactly the comparison the sweep exists to make.
func TestDuplicatesSurviveAveraging(t *testing.T) {
	e := &experiment{results: []expResult{
		{Arm: "loop off", Seed: 1, Messages: 2, DupeRx: 100, WorstDupe: 9},
		{Arm: "loop off", Seed: 2, Messages: 2, DupeRx: 60, WorstDupe: 5},
		{Arm: "loop strict", Seed: 1, Messages: 2, DupeRx: 10, WorstDupe: 2},
		{Arm: "loop strict", Seed: 2, Messages: 2, DupeRx: 6, WorstDupe: 2},
	}}
	sums := e.summarise()
	if len(sums) != 2 {
		t.Fatalf("got %d arms, want 2", len(sums))
	}
	off, strict := sums[0], sums[1]
	if off.Dupe != 80 {
		t.Errorf("loop off averaged %.0f duplicates, want 80", off.Dupe)
	}
	if strict.Dupe != 8 {
		t.Errorf("loop strict averaged %.0f duplicates, want 8", strict.Dupe)
	}
	// Per message, so arms that delivered different numbers of messages stay
	// comparable - an arm that got half the traffic out would otherwise look
	// like it had halved the duplication.
	if off.DupePerMsg != 40 {
		t.Errorf("loop off averaged %.0f duplicates per message, want 40", off.DupePerMsg)
	}
	// The tail, not the mean: one pair of neighbours reflecting a packet between
	// them is a real failure that an average over 200 nodes hides completely.
	if off.WorstDupe != 7 {
		t.Errorf("loop off worst duplicate averaged %.0f, want 7", off.WorstDupe)
	}
}

// A run with no messages must not turn into a division by zero, which is the
// shape every "nothing relayed" run has.
func TestDuplicatesPerMessageSurvivesAnEmptyRun(t *testing.T) {
	e := &experiment{results: []expResult{
		{Arm: "a", Seed: 1, Messages: 0, DupeRx: 0},
		{Arm: "a", Seed: 2, Messages: 4, DupeRx: 8},
	}}
	sums := e.summarise()
	if got := sums[0].DupePerMsg; got != 1 {
		t.Errorf("duplicates per message %.2f, want 1 (0 from the empty run, 2 from the other)", got)
	}
}
