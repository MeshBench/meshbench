package engine

import (
	"testing"
	"time"
)

// The attach budget grows with how many nodes are brought up at once, so a
// full worker pool contending for the CPU does not read a slow scheduler as a
// hung process; and it never drops below a floor a single node can rely on.
func TestAttachBudgetScalesWithContention(t *testing.T) {
	// A dozen at once - the pool cap - must get far more than a lone node's ten
	// seconds, or the last to be scheduled flakes on a loaded runner.
	if got := attachBudget(12); got != 60*time.Second {
		t.Errorf("twelve workers get %v, want 60s", got)
	}
	// A larger pool scales linearly rather than saturating.
	if got := attachBudget(20); got != 100*time.Second {
		t.Errorf("twenty workers get %v, want 100s", got)
	}
	// The floor holds when there is almost no contention.
	for _, w := range []int{0, 1, 5, 6} {
		if got := attachBudget(w); got < 30*time.Second {
			t.Errorf("%d workers get %v, below the 30s floor", w, got)
		}
	}
}
