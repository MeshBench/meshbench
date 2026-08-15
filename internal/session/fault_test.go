package session

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/gui/state"
)

// bridgeLinks is the network the plan's own acceptance test names: two
// clusters (0,1 and 3,4) joined only through node 2. Every link margin is
// symmetric here - the asymmetric case gets its own test below.
func bridgeLinks() []state.Link {
	sym := func(a, b int, m float64) state.Link {
		return state.Link{A: a, B: b, MarginDB: m, AtoB: m, BtoA: m, Known: true}
	}
	return []state.Link{
		sym(0, 1, 10), // within cluster A
		sym(0, 2, 5),  // A to the bridge
		sym(2, 3, 5),  // the bridge to B
		sym(3, 4, 10), // within cluster B
	}
}

func TestReachCountsWithTheBridgeUp(t *testing.T) {
	links := bridgeLinks()
	out, in := reachCounts(links, 0, nil)
	if out != 4 || in != 4 {
		t.Fatalf("node 0 with the bridge up: out=%d in=%d, want 4 and 4 (every other node)", out, in)
	}
}

// TestReachCountsPartitionsWhenTheBridgeFails is the plan's own primary
// acceptance test: a failure on the bridge network partitions it.
func TestReachCountsPartitionsWhenTheBridgeFails(t *testing.T) {
	links := bridgeLinks()
	down := map[int]bool{2: true}
	out, in := reachCounts(links, 0, down)
	if out != 1 || in != 1 {
		t.Fatalf("node 0 with the bridge down: out=%d in=%d, want 1 and 1 (only node 1, in its own cluster)", out, in)
	}
	// The other side of the partition sees the same thing, symmetrically.
	out, in = reachCounts(links, 4, down)
	if out != 1 || in != 1 {
		t.Fatalf("node 4 with the bridge down: out=%d in=%d, want 1 and 1", out, in)
	}
}

// TestReachCountsRecoversWhenRestored is the second half of the same
// acceptance test: restore repairs it.
func TestReachCountsRecoversWhenRestored(t *testing.T) {
	links := bridgeLinks()
	before := map[int]bool{2: true}
	outDown, inDown := reachCounts(links, 0, before)
	outUp, inUp := reachCounts(links, 0, nil)
	if outDown >= outUp || inDown >= inUp {
		t.Fatalf("restoring the bridge did not raise reachability: down=%d/%d up=%d/%d",
			outDown, inDown, outUp, inUp)
	}
	if outUp != 4 || inUp != 4 {
		t.Fatalf("restored reach = %d/%d, want back to 4/4", outUp, inUp)
	}
}

// TestReachCountsIsAsymmetric is CLAUDE.md's own risk, made concrete: a mast
// heard by a handheld it cannot hear back must report two different numbers,
// not the one that happens to be easiest to compute.
func TestReachCountsIsAsymmetric(t *testing.T) {
	// 0 -> 1 closes; 1 -> 0 does not (a mast and a handheld).
	links := []state.Link{
		{A: 0, B: 1, MarginDB: -5, AtoB: 8, BtoA: -3, Known: true},
	}
	out, in := reachCounts(links, 0, nil)
	if out != 1 {
		t.Fatalf("node 0 should flood-reach node 1 (AtoB=8): got out=%d", out)
	}
	if in != 0 {
		t.Fatalf("node 0 should not be reachable from node 1 (BtoA=-3): got in=%d", in)
	}
	// From node 1's side, the two are swapped.
	out, in = reachCounts(links, 1, nil)
	if out != 0 || in != 1 {
		t.Fatalf("node 1: out=%d in=%d, want 0 and 1 (the asymmetry the other way round)", out, in)
	}
}

func TestReachCountsOfADownNodeIsZero(t *testing.T) {
	links := bridgeLinks()
	out, in := reachCounts(links, 2, map[int]bool{2: true})
	if out != 0 || in != 0 {
		t.Fatalf("a down node's own reach: out=%d in=%d, want 0 and 0", out, in)
	}
}

func TestUndeliveredCost(t *testing.T) {
	events := []state.Event{
		{Kind: "tx", AtMs: 1000, MessageID: 1},
		{Kind: "tx", AtMs: 1500, MessageID: 2},
		{Kind: "rx", AtMs: 1600, MessageID: 1}, // 1 got through
		{Kind: "tx", AtMs: 5000, MessageID: 3}, // outside the window
	}
	got := undeliveredCost(events, 900, 2000)
	if got != 1 {
		t.Fatalf("undeliveredCost = %d, want 1 (message 2, never received; message 3 is outside the window)", got)
	}
}

func TestNodeIndex(t *testing.T) {
	nodes := []state.Node{{Name: "A"}, {Name: "B"}}
	if i, ok := nodeIndex(nodes, "B"); !ok || i != 1 {
		t.Fatalf("nodeIndex(B) = %d, %v, want 1, true", i, ok)
	}
	if _, ok := nodeIndex(nodes, "nope"); ok {
		t.Fatal("nodeIndex found a node that is not there")
	}
}
