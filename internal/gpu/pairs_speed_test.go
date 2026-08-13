package gpu

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/A13xB0/meshcoresim/internal/coverage"
)

// What the GPU is worth, on the network that made this necessary.
//
// Not a benchmark for its own sake: the claim being made to somebody choosing
// a setting is "this is faster", and a claim about performance with no number
// behind it is the kind this project does not make.
func TestPairLossIsWorthIt(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}
	d, err := Open()
	if err != nil {
		t.Skipf("no usable GPU here: %v", err)
	}
	defer d.Close()

	const w, h = 4096, 4096
	g := coverage.HeightGrid{
		South: 56.0, North: 56.9, West: -3.9, East: -2.4,
		W: w, H: h, Heights: make([]float32, w*h),
	}
	rng := rand.New(rand.NewSource(9001))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x)/w, float64(y)/h
			g.Heights[y*w+x] = float32(300*math.Sin(fx*7)*math.Cos(fy*5) + 180*math.Sin(fy*11))
		}
	}
	// The size that hurts: 311 nodes is 48,205 pairs.
	nodes := make([]coverage.PairNode, 0, 311)
	for i := 0; i < 311; i++ {
		nodes = append(nodes, coverage.PairNode{
			Lat:  g.South + (g.North-g.South)*rng.Float64(),
			Lon:  g.West + (g.East-g.West)*rng.Float64(),
			AGLm: 2 + 30*rng.Float64(),
		})
	}
	p := coverage.PairParams{FreqMHz: 869.618, StepM: 60, StepsCap: 256}

	start := time.Now()
	got, err := d.PairLoss(g, nodes, p)
	onGPU := time.Since(start)
	if err != nil {
		t.Fatalf("pair loss on the GPU: %v", err)
	}

	start = time.Now()
	want := coverage.PairLossCPU(g, nodes, p)
	onCPU := time.Since(start)

	// Still the same answer at this size, which is the point of measuring the
	// two together rather than only timing them.
	n := len(nodes)
	worst := 0.0
	for a := 0; a < n; a++ {
		for b := a + 1; b < n; b++ {
			i := a*n + b
			if want[i] == coverage.NoDataLoss || got[i] == coverage.NoDataLoss {
				continue
			}
			if d := math.Abs(float64(got[i] - want[i])); d > worst {
				worst = d
			}
		}
	}
	t.Logf("%d pairs: GPU %v, one core %v (%.1fx), worst disagreement %.4f dB",
		n*(n-1)/2, onGPU.Round(time.Millisecond), onCPU.Round(time.Millisecond),
		float64(onCPU)/float64(onGPU), worst)
	if worst > 0.1 {
		t.Errorf("worst disagreement %.3f dB at 311 nodes", worst)
	}
}
