package gpu

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/propagation"
)

// syntheticProfiles packs pairs with real relief in them: hills between the
// endpoints, because a flat profile would pass with the diffraction term
// deleted.
func syntheticProfiles(nPairs int, seed int64) propagation.PairProfiles {
	rng := rand.New(rand.NewSource(seed))
	var p propagation.PairProfiles
	for i := 0; i < nPairs; i++ {
		distM := 2000 + rng.Float64()*60000
		steps := int(distM / 60)
		if steps > 256 {
			steps = 256
		}
		hs := make([]float32, steps+1)
		base := rng.Float64() * 200
		amp := 50 + rng.Float64()*400
		phase := rng.Float64() * math.Pi
		for k := range hs {
			f := float64(k) / float64(steps)
			hs[k] = float32(base + amp*math.Sin(f*7+phase)*math.Cos(f*3))
		}
		p.Add(hs, distM, 2+rng.Float64()*28, 2+rng.Float64()*28)
	}
	return p
}

// The pairs kernel and its CPU twin have to agree (ADR-0004).
func TestPairLossMatchesCPU(t *testing.T) {
	d, err := Open()
	if err != nil {
		t.Skipf("no usable GPU here: %v", err)
	}
	defer d.Close()

	p := syntheticProfiles(600, 9001)
	got, err := d.PairProfileLoss(p, 869.618)
	if err != nil {
		t.Fatalf("pair loss on the GPU: %v", err)
	}
	want := propagation.ProfilePairLossCPU(p, 869.618)

	worst, at := 0.0, -1
	for i := range want {
		if d := math.Abs(float64(got[i] - want[i])); d > worst {
			worst, at = d, i
		}
	}
	// A tenth of a decibel: the same arithmetic in the same order, so the
	// only difference should be single-precision transcendentals.
	if worst > 0.1 {
		t.Errorf("worst disagreement %.3f dB at pair %d over %d pairs",
			worst, at, len(want))
	}
	t.Logf("%d pairs compared, worst disagreement %.4f dB", len(want), worst)
}

// What the GPU is worth at the size that made this necessary.
func TestPairLossIsWorthIt(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}
	d, err := Open()
	if err != nil {
		t.Skipf("no usable GPU here: %v", err)
	}
	defer d.Close()

	p := syntheticProfiles(48205, 9001)
	start := time.Now()
	got, err := d.PairProfileLoss(p, 869.618)
	onGPU := time.Since(start)
	if err != nil {
		t.Fatalf("pair loss on the GPU: %v", err)
	}
	start = time.Now()
	want := propagation.ProfilePairLossCPU(p, 869.618)
	onCPU := time.Since(start)

	worst := 0.0
	for i := range want {
		if d := math.Abs(float64(got[i] - want[i])); d > worst {
			worst = d
		}
	}
	t.Logf("%d pairs: GPU %v, one core %v (%.1fx), worst disagreement %.4f dB",
		len(want), onGPU.Round(time.Millisecond), onCPU.Round(time.Millisecond),
		float64(onCPU)/float64(onGPU), worst)
	if worst > 0.1 {
		t.Errorf("worst disagreement %.3f dB", worst)
	}
}
