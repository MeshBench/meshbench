package engine_test

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/engine"
)

// TestSetPositionInvalidatesOnlyThatNodesPairs is plan §13's own acceptance
// criterion for cache granularity, made direct: a moving node's own pairs
// must be dropped, and nobody else's may be - the difference between
// InvalidateLinks (drops everything) and SetPosition (drops exactly the
// pairs that changed).
func TestSetPositionInvalidatesOnlyThatNodesPairs(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10})
	const n = 6
	for i := 0; i < n; i++ {
		e.Add(node(rname(i), 56.70+float64(i)*0.05, -3.90, 22), nil)
	}
	// Warm every pair.
	total := 0
	for a := 0; a < n; a++ {
		for b := a + 1; b < n; b++ {
			e.PathLossForTest(a, b)
			total++
		}
	}
	if got := e.LinkCacheSizeForTest(); got != total {
		t.Fatalf("warmed %d pairs, cache holds %d", total, got)
	}

	if ok := e.SetPosition(rname(0), 57.10, -3.50); !ok {
		t.Fatal("SetPosition did not find the node")
	}
	// Node 0 is in n-1 of the C(n,2) pairs; those, and only those, must be
	// gone.
	wantRemoved := n - 1
	if got := e.LinkCacheSizeForTest(); got != total-wantRemoved {
		t.Fatalf("after moving one node: cache holds %d, want %d (removed %d of %d, not the whole cache)",
			got, total-wantRemoved, wantRemoved, total)
	}
	// A pair with neither end moved must still answer from the cache -
	// checked functionally, since a correct recompute of unmoved geometry
	// would return the same number either way; what LinkCacheSizeForTest
	// above already proved is that it did not have to.
	if _, ok := e.PathLossForTest(2, 3); !ok {
		t.Fatal("an untouched pair stopped resolving after an unrelated move")
	}

	e.InvalidateLinks()
	if got := e.LinkCacheSizeForTest(); got != 0 {
		t.Fatalf("InvalidateLinks left %d pairs cached, want 0 (the full-drop this is compared against)", got)
	}
}

// TestSetPositionChangesThePathLoss is the other half: a move has to be
// visible, not just cheap.
func TestSetPositionChangesThePathLoss(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10})
	e.Add(node("a", 56.70, -3.90, 22), nil)
	e.Add(node("b", 56.701, -3.90, 22), nil) // starts a hundred metres from a

	before, ok := e.PathLossForTest(0, 1)
	if !ok {
		t.Fatal("co-located nodes should resolve a path loss")
	}
	if !e.SetPosition("b", 58.90, -1.20) { // hundreds of km away
		t.Fatal("SetPosition did not find node b")
	}
	after, ok := e.PathLossForTest(0, 1)
	if !ok {
		t.Fatal("the moved pair should still resolve, just at a much higher loss")
	}
	if after <= before {
		t.Fatalf("moving b hundreds of km away should raise the path loss: before=%.1f after=%.1f", before, after)
	}
}

func rname(i int) string {
	return string(rune('a' + i))
}
