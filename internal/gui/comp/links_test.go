package comp

import (
	"testing"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
)

// at builds points for nodes at the given positions, as project() would.
func at(pos ...[2]float64) []projected {
	nodes := make([]state.Node, len(pos))
	out := make([]projected, len(pos))
	for i, p := range pos {
		nodes[i] = state.Node{Lat: p[0], Lon: p[1]}
		out[i] = projected{n: &nodes[i]}
	}
	return out
}

// Two nodes 5 km apart are linked; two 200 km apart are not. Without this the
// cache could be perfectly consistent and perfectly wrong.
func TestPairsAreTheOnesInRange(t *testing.T) {
	var c linkCache
	pts := at([2]float64{56.0, -3.0}, [2]float64{56.045, -3.0}, [2]float64{58.0, -3.0})
	got := c.get(pts)
	if len(got) != 1 {
		t.Fatalf("got %d pairs, want 1: %v", len(got), got)
	}
	if got[0] != [2]int{0, 1} {
		t.Fatalf("linked %v, want the two close ones", got[0])
	}
}

// The point of the cache: a second call with unchanged positions must not
// recompute. Proved by mutating the cached slice and seeing the mutation come
// back, which can only happen if it was not rebuilt.
func TestUnchangedPositionsReuseTheCache(t *testing.T) {
	var c linkCache
	pts := at([2]float64{56.0, -3.0}, [2]float64{56.045, -3.0})
	first := c.get(pts)
	if len(first) != 1 {
		t.Fatalf("setup: got %d pairs", len(first))
	}
	first[0] = [2]int{9, 9}

	if again := c.get(pts); again[0] != [2]int{9, 9} {
		t.Fatal("the pairs were recomputed for a network that had not moved")
	}
}

// And the other half: a node that moves must invalidate it. A cache that
// never rebuilds is worse than no cache, because the map would then be
// permanently wrong instead of merely slow.
func TestAMovedNodeInvalidatesTheCache(t *testing.T) {
	var c linkCache
	pts := at([2]float64{56.0, -3.0}, [2]float64{56.045, -3.0})
	if len(c.get(pts)) != 1 {
		t.Fatal("setup")
	}
	pts[1].n.Lat = 58.0

	if got := c.get(pts); len(got) != 0 {
		t.Fatalf("got %d pairs after the node moved out of range, want 0", len(got))
	}
}

// Simulated time advancing is not a reason to recompute. This is why the cache
// is keyed on positions rather than on Snapshot.Seq, which increases on every
// tick and would throw the cache away sixty times a second.
func TestTimePassingDoesNotInvalidateTheCache(t *testing.T) {
	pts := at([2]float64{56.0, -3.0}, [2]float64{56.045, -3.0})
	before := fingerprint(pts)
	// Everything about a node that is not where it is.
	pts[0].n.Name = "renamed"
	pts[0].n.Selected = true
	pts[0].x, pts[0].y = 900, 900
	if fingerprint(pts) != before {
		t.Fatal("the fingerprint changed for something that cannot change who hears whom")
	}
}

// A node appearing or disappearing changes the answer, and the pair indices
// would otherwise point at the wrong nodes.
func TestANodeAppearingInvalidatesTheCache(t *testing.T) {
	pts := at([2]float64{56.0, -3.0}, [2]float64{56.045, -3.0})
	if fingerprint(pts) == fingerprint(pts[:1]) {
		t.Fatal("adding a node did not change the fingerprint")
	}
}
