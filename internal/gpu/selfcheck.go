package gpu

import (
	"fmt"
	"math"

	"github.com/MeshBench/meshbench/internal/coverage"
)

// SelfCheck runs a small problem on the device and compares the answer with
// the processor's, refusing the device if they disagree.
//
// This exists because a backend can be present, report every capability it
// needs, compile the shaders, run them, and still be wrong. Windows is the
// case that proved it: on a d3d12 adapter the coverage kernel returned cells
// that differ from the CPU twin by nearly ten decibels - not a crash, not a
// missing feature, just a different answer. Metal and Vulkan agree with the
// processor; that one does not.
//
// A wrong link budget is worse than a slow one, and the difference is
// invisible: the map fills in, the numbers look plausible, and every claim
// made from them is off by an amount nobody can see. So the device has to
// earn its place at startup, on a problem small enough to be free.
//
// ADR-0004 already says the CPU path is a maintained oracle rather than a
// fallback. This is that oracle used at runtime rather than only in tests.
func (d *Device) SelfCheck() error {
	if err := d.checkCoverage(); err != nil {
		return err
	}
	return d.checkPairs()
}

// tolerance is the same one the equivalence tests justify: the two paths sum
// the same terms in a different order, so they may differ in the last place
// and nowhere else. A real disagreement is orders of magnitude larger - the
// d3d12 divergence was 9.8 dB.
const tolerance = 0.5

func (d *Device) checkCoverage() error {
	g := checkGrid()
	p := coverage.GridLossParams{
		StLat: 56.75, StLon: -3.7, StAltM: 620,
		RasterW: 24, RasterH: 24,
		South: 56.6, North: 56.9, West: -3.9, East: -3.5,
		RemoteHeightM: 1.5, FreqMHz: 869.525, Steps: 60,
	}
	want := coverage.GridLossCPU(g, p)
	got, err := d.CoverageGridLoss(g, p)
	if err != nil {
		return fmt.Errorf("coverage kernel: %w", err)
	}
	return compare("coverage", got, want)
}

func (d *Device) checkPairs() error {
	var pp coverage.PairProfiles
	// Three profiles: flat, a ridge in the middle, and a slope. Enough shape
	// that a kernel which ignores the terrain still gets caught.
	heights := make([]float32, 64)
	pp.Add(heights, 8000, 10, 1.5)
	for i := range heights {
		if i > 24 && i < 40 {
			heights[i] = 220
		}
	}
	pp.Add(heights, 8000, 10, 1.5)
	for i := range heights {
		heights[i] = float32(i) * 4
	}
	pp.Add(heights, 8000, 10, 1.5)

	want := coverage.ProfilePairLossCPU(pp, 869.525)
	got, err := d.PairProfileLoss(pp, 869.525)
	if err != nil {
		return fmt.Errorf("pairs kernel: %w", err)
	}
	return compare("pairs", got, want)
}

func compare(what string, got, want []float32) error {
	if len(got) != len(want) {
		return fmt.Errorf("%s kernel returned %d values, the processor %d",
			what, len(got), len(want))
	}
	worst, at := 0.0, -1
	for i := range want {
		// No-data cells are a refusal on both sides rather than a number, so
		// they are compared as a pair of refusals.
		if math.IsInf(float64(want[i]), 1) || want[i] == coverage.NoDataLoss {
			continue
		}
		if d := math.Abs(float64(got[i] - want[i])); d > worst {
			worst, at = d, i
		}
	}
	if worst > tolerance {
		return fmt.Errorf(
			"%s kernel disagrees with the processor by %.1f dB at sample %d - "+
				"the device computes a different answer, not a slower one",
			what, worst, at)
	}
	return nil
}

// checkGrid is a small piece of ground with relief in it: flat terrain would
// let a kernel that never reads the heightmap pass.
func checkGrid() coverage.HeightGrid {
	const n = 48
	g := coverage.HeightGrid{
		W: n, H: n,
		South: 56.6, North: 56.9, West: -3.9, East: -3.5,
		Heights: make([]float32, n*n),
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			fx, fy := float64(x)/n, float64(y)/n
			g.Heights[y*n+x] = float32(300 +
				400*math.Exp(-((fx-0.5)*(fx-0.5)+(fy-0.5)*(fy-0.5))*12) +
				60*math.Sin(fx*9))
		}
	}
	return g
}
