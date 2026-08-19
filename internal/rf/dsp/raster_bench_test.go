package dsp

import (
	"fmt"
	"math"
	"testing"
)

// MSIM-3 part two: the raster workload is a different shape from the RF loop.
// A coverage pass is millions of independent terrain profile walks, which is the
// classic case for a GPU — measure it separately rather than assuming the RF
// number generalises.
func TestRasterCeiling(t *testing.T) {
	if testing.Short() {
		t.Skip("benchmark; run without -short")
	}
	// Stand-in for a profile walk: N terrain samples with a diffraction-ish
	// reduction per cell. Deliberately arithmetic-only — it measures the shape
	// of the workload, not our not-yet-written terrain code.
	profile := func(steps int) float64 {
		worst := 0.0
		h := 0.0
		for i := 0; i < steps; i++ {
			x := float64(i)
			h = math.Sin(x*0.013)*120 + math.Cos(x*0.007)*80
			bulge := x * (float64(steps) - x) / (2 * 8495000)
			v := h - bulge
			if v > worst {
				worst = v
			}
		}
		return worst
	}

	for _, cells := range []int{100_000, 1_000_000} {
		const steps = 200 // profile samples per cell
		t0 := timeNow()
		acc := 0.0
		for c := 0; c < cells; c++ {
			acc += profile(steps)
		}
		el := float64(timeNow()-t0) / 1e9
		fmt.Printf("  %9d cells x %d steps: %6.2f s single-core  (%.1f M profile-steps/s)\n",
			cells, steps, el, float64(cells)*steps/el/1e6)
		_ = acc
	}
	fmt.Printf("  a 3.1 M-cell raster (20 m/px) therefore costs roughly:\n")
	fmt.Printf("    single core  ~%.0f s     12 cores ~%.0f s\n", 3.1e6/1e6*10.0, 3.1e6/1e6*10.0/12)
}
