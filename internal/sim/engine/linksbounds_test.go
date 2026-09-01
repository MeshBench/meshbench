package engine

import "testing"

// A pair this engine has no nodes for is refused, not indexed.
//
// The caller that produced such an index is the defect and is fixed where it
// lives, but the cost of being wrong here is not a wrong number: pathLoss runs
// on warm workers, and a bounds panic on a goroutine takes the process and
// whatever was unsaved in it. So the floor under that is an answer of "no",
// which every caller of a path loss already handles.
func TestAnOutOfRangePairIsRefusedRatherThanPanicking(t *testing.T) {
	e := New(&countingTerrain{}, Config{})
	e.Add(profNode("Alpha", -3.2), nil)
	e.Add(profNode("Beta", -3.1), nil)

	for _, pair := range [][2]int{{0, 2}, {2, 0}, {7, 9}, {-1, 0}} {
		if _, ok := e.pathLoss(pair[0], pair[1]); ok {
			t.Errorf("pair %d,%d of a two-node engine was answered for", pair[0], pair[1])
		}
	}
	// And the pair it does have is still measured, so the guard refuses only
	// what it should.
	if _, ok := e.pathLoss(0, 1); !ok {
		t.Error("the one real pair went unanswered")
	}
}
