package comp

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// Every kind is painted, and in a fixed order.
//
// byKind is a map and Go randomises map iteration, so ranging it painted the
// kinds in a different order on every frame: two nodes close enough to overlap
// swapped which was on top several times a second, which reads as the map
// flickering. A kind missing from this list would silently stop being drawn at
// all, which is why the count is checked rather than assumed.
func TestEveryKindIsPaintedExactlyOnce(t *testing.T) {
	seen := map[theme.NodeKind]int{}
	for _, k := range kindPaintOrder {
		seen[k]++
	}
	for k := theme.SimpleRepeater; k <= theme.Emitter; k++ {
		if seen[k] != 1 {
			t.Errorf("kind %d appears %d times in the paint order, want 1", k, seen[k])
		}
	}
	if len(kindPaintOrder) != len(seen) {
		t.Errorf("the paint order holds %d entries for %d kinds", len(kindPaintOrder), len(seen))
	}
}

// Companions last means companions on top: they are what somebody is looking
// for on a crowded map, and there are far fewer of them than repeaters.
func TestCompanionsArePaintedOnTop(t *testing.T) {
	last := kindPaintOrder[len(kindPaintOrder)-1]
	if last != theme.Companion {
		t.Errorf("the last kind painted is %d, want Companion (%d)", last, theme.Companion)
	}
	if kindPaintOrder[0] != theme.SimpleRepeater {
		t.Errorf("the first kind painted is %d, want SimpleRepeater", kindPaintOrder[0])
	}
}
