package gpu

import (
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/propagation"
)

// hilly builds a synthetic height grid with real structure: ridges tall
// enough to diffract, valleys low enough to clear, and a hole of missing
// data — the three cases the kernel must handle.
func hilly() propagation.HeightGrid {
	const w, h = 128, 128
	g := propagation.HeightGrid{
		South: 56.5, North: 57.0, West: -4.2, East: -3.2,
		W: w, H: h, Heights: make([]float32, w*h),
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x)/w, float64(y)/h
			elev := 200 + 400*math.Sin(fx*7)*math.Sin(fy*5) + 150*math.Sin(fx*23)
			if elev < 0 {
				elev = 0
			}
			g.Heights[y*w+x] = float32(elev)
			// A patch of missing tiles in one corner.
			if x > 115 && y > 115 {
				g.Heights[y*w+x] = propagation.NoDataHeight
			}
		}
	}
	return g
}

// The ADR-0004 harness for the coverage kernel: the CPU twin is the oracle,
// and the GPU must agree with it everywhere — including on the no-data cells,
// where "agree" means both refuse rather than both inventing ground.
func TestCoverageKernelMatchesCPU(t *testing.T) {
	d, err := Open()
	if err != nil {
		t.Skipf("no GPU available: %v", err)
	}
	defer d.Close()
	t.Logf("GPU: %s (%s)", d.Name, d.Backend)

	g := hilly()
	p := propagation.GridLossParams{
		StLat: 56.75, StLon: -3.7, StAltM: 620,
		RasterW: 96, RasterH: 96,
		South: 56.5, North: 57.0, West: -4.2, East: -3.2,
		RemoteHeightM: 1.5, FreqMHz: 869.525, Steps: 200,
	}
	want := propagation.GridLossCPU(g, p)
	got, err := d.CoverageGridLoss(g, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("gpu returned %d cells, cpu %d", len(got), len(want))
	}

	worst, worstAt := 0.0, -1
	mismatchedNoData := 0
	for i := range want {
		cpuNoData := want[i] > 1e30
		gpuNoData := got[i] > 1e30
		if cpuNoData != gpuNoData {
			mismatchedNoData++
			continue
		}
		if cpuNoData {
			continue
		}
		if diff := math.Abs(float64(got[i] - want[i])); diff > worst {
			worst, worstAt = diff, i
		}
	}
	if mismatchedNoData > 0 {
		t.Errorf("%d cells disagree about whether the terrain answered", mismatchedNoData)
	}
	// f32 against f64 across a 200-step accumulation: fractions of a dB is
	// honest agreement; whole dB is a wrong kernel.
	if worst > 0.5 {
		t.Fatalf("worst disagreement %.3f dB at cell %d - the twins have diverged", worst, worstAt)
	}
	t.Logf("worst disagreement %.4f dB over %d cells", worst, len(want))
}
