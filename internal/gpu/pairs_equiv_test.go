package gpu

import (
	"math"
	"math/rand"
	"testing"

	"github.com/A13xB0/meshcoresim/internal/coverage"
)

// The pair kernel and its CPU twin have to agree.
//
// A wrong kernel here does not crash: it produces a plausible link matrix and
// slightly wrong margins, and nobody notices for months. So the two are held
// together by construction, over terrain with real relief in it rather than a
// flat plane where a diffraction bug cannot show.
func TestPairLossMatchesCPU(t *testing.T) {
	d, err := Open()
	if err != nil {
		t.Skipf("no usable GPU here: %v", err)
	}
	defer d.Close()

	const w, h = 96, 96
	g := coverage.HeightGrid{
		South: 56.0, North: 56.6, West: -3.9, East: -2.6,
		W: w, H: h, Heights: make([]float32, w*h),
	}
	// Ridges and a valley: a hill between two nodes is the whole point of the
	// calculation, and a grid of zeroes would pass with the diffraction term
	// deleted.
	rng := rand.New(rand.NewSource(9001))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x)/w, float64(y)/h
			g.Heights[y*w+x] = float32(
				300*math.Sin(fx*7)*math.Cos(fy*5) +
					180*math.Sin(fy*11) +
					40*rng.Float64())
		}
	}

	nodes := make([]coverage.PairNode, 0, 24)
	for i := 0; i < 24; i++ {
		nodes = append(nodes, coverage.PairNode{
			Lat:  g.South + (g.North-g.South)*rng.Float64(),
			Lon:  g.West + (g.East-g.West)*rng.Float64(),
			AGLm: 2 + 30*rng.Float64(),
		})
	}
	p := coverage.PairParams{FreqMHz: 869.618, StepM: 60, StepsCap: 256}

	got, err := d.PairLoss(g, nodes, p)
	if err != nil {
		t.Fatalf("pair loss on the GPU: %v", err)
	}
	want := coverage.PairLossCPU(g, nodes, p)
	if len(got) != len(want) {
		t.Fatalf("got %d results, the twin produced %d", len(got), len(want))
	}

	n := len(nodes)
	worst, worstAt := 0.0, [2]int{}
	pairs := 0
	for a := 0; a < n; a++ {
		for b := a + 1; b < n; b++ {
			i := a*n + b
			// No data is no data on both sides, or the disagreement is about
			// coverage of the grid rather than about radio.
			if want[i] == coverage.NoDataLoss || got[i] == coverage.NoDataLoss {
				if want[i] != got[i] {
					t.Fatalf("pair %d-%d: one side has no data and the other does: cpu %v gpu %v",
						a, b, want[i], got[i])
				}
				continue
			}
			pairs++
			if d := math.Abs(float64(got[i] - want[i])); d > worst {
				worst, worstAt = d, [2]int{a, b}
			}
		}
	}
	if pairs == 0 {
		t.Fatal("no pair had data on either side, so nothing was compared")
	}
	// A tenth of a decibel. The two run the same arithmetic in the same
	// order, so the only difference should be the GPU's single-precision
	// transcendentals against Go's.
	if worst > 0.1 {
		t.Errorf("worst disagreement %.3f dB, at pair %d-%d, over %d pairs",
			worst, worstAt[0], worstAt[1], pairs)
	}
	t.Logf("%d pairs compared, worst disagreement %.4f dB", pairs, worst)
}
