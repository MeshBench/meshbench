package float

import (
	"sync"
	"testing"
)

// Every window opens on a goroutine of its own - popOut and the node windows
// both run their event loop in one - so two opened together ask for a spot at
// the same moment. The counter behind NextSpot is shared, and an unguarded one
// is a data race that Go does not merely tolerate badly: it kills the process,
// taking the main window and every panel with it.
//
// Only the detector sees this, and the detector runs on tags and on request
// rather than on push, so the test exists to be run by
// `go test -race ./internal/ui/float/` when this file changes.
func TestNextSpotIsAskedFromEveryWindowAtOnce(t *testing.T) {
	const windows = 8
	var wg sync.WaitGroup
	spots := make([]Spot, windows)
	for i := range spots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			spots[i] = NextSpot()
		}(i)
	}
	wg.Wait()

	// And the stagger still staggers: eight windows land on eight slots, not
	// on top of one another.
	seen := map[Spot]bool{}
	for _, s := range spots {
		seen[s] = true
	}
	if len(seen) != windows {
		t.Errorf("%d windows took %d distinct spots: %v", windows, len(seen), spots)
	}
}
