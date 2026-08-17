package basemap_test

import (
	"math"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/basemap"
)

// The bug this file exists for: a viewport zoomed right out asks for a box far
// bigger than the planet, and the answer used to be an unbounded loop.
//
// Timed, because the failure was never a wrong number - it was a function that
// did not come back, taking the machine with it.
func TestZoomingOutCannotAskForMoreThanTheWorld(t *testing.T) {
	for _, tc := range []struct {
		name       string
		s, n, w, e float64
		zoom       int
	}{
		{"the whole planet and then some", -450e6, 450e6, -1.4e9, 1.4e9, 1},
		{"past the poles", -169, 281, -719, 711, 1},
		{"a hemisphere of longitude", -85, 85, -720, 720, 3},
		{"inverted, as a viewport can be", 60, -60, 20, -20, 2},
		{"NaN, which is what the projection produced", math.NaN(), math.NaN(), math.NaN(), math.NaN(), 4},
		{"infinite", math.Inf(-1), math.Inf(1), math.Inf(-1), math.Inf(1), 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			done := make(chan int, 1)
			go func() { done <- len(basemap.TilesFor(tc.s, tc.n, tc.w, tc.e, tc.zoom)) }()
			select {
			case got := <-done:
				world := int(math.Exp2(float64(tc.zoom * 2)))
				if got > world {
					t.Errorf("%d tiles at zoom %d, and the world holds %d",
						got, tc.zoom, world)
				}
				if got == 0 {
					t.Error("a box covering the world should cover some tiles")
				}
			case <-time.After(5 * time.Second):
				// Deliberately fatal rather than a skip: this is the bug.
				t.Fatal("TilesFor did not return")
			}
		})
	}
}

// Clamping the world must not change what an ordinary view asks for.
func TestAnOrdinaryViewIsUnchanged(t *testing.T) {
	// A few hundred metres across the middle of Scotland, at street zoom.
	got := basemap.TilesFor(56.4770, 56.4800, -3.1300, -3.1250, 15)
	if len(got) == 0 || len(got) > 12 {
		t.Fatalf("%d tiles for a small view, wanted a handful", len(got))
	}
	for _, xy := range got {
		if xy[0] < 0 || xy[1] < 0 || xy[0] > 32767 || xy[1] > 32767 {
			t.Errorf("tile %v is off the zoom-15 grid", xy)
		}
	}
}

// Every tile in every answer has to be one that exists, or the fetcher builds
// URLs for tiles no server has and the map goes blank where it should not.
func TestEveryTileIsOnTheGrid(t *testing.T) {
	for zoom := 0; zoom <= 12; zoom++ {
		last := int(math.Exp2(float64(zoom))) - 1
		for _, xy := range basemap.TilesFor(-90, 90, -180, 180, zoom) {
			if xy[0] < 0 || xy[0] > last || xy[1] < 0 || xy[1] > last {
				t.Fatalf("zoom %d produced tile %v, off a 0..%d grid", zoom, xy, last)
			}
		}
	}
}

// The world, exactly - at zoom 1 that is four tiles and not three or six.
func TestTheWholeWorldIsTheWholeWorld(t *testing.T) {
	if got := len(basemap.TilesFor(-85, 85, -180, 180, 1)); got != 4 {
		t.Errorf("the world at zoom 1 is %d tiles, wanted 4", got)
	}
	if got := len(basemap.TilesFor(-85, 85, -180, 180, 0)); got != 1 {
		t.Errorf("the world at zoom 0 is %d tiles, wanted 1", got)
	}
}

func TestAnAbsurdZoomIsRefusedRatherThanAttempted(t *testing.T) {
	if got := basemap.TilesFor(-85, 85, -180, 180, 40); got != nil {
		t.Errorf("zoom 40 returned %d tiles; 2^40 square does not fit anywhere", len(got))
	}
	if got := basemap.TilesFor(-85, 85, -180, 180, -1); got != nil {
		t.Errorf("a negative zoom returned %d tiles", len(got))
	}
}
