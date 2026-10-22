package gpu

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/rf/propagation"
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
	// Two co-located pairs among them. Zero distance is where log10 stops
	// having an answer, and a kernel that returns -Inf there hands the caller
	// an infinite margin: the one disagreement between the twins that would
	// look like the best link on the map rather than like a fault.
	flat := make([]float32, 40)
	for i := range flat {
		flat[i] = 100
	}
	p.Add(flat, 0, 10, 1.5)
	p.Add(flat, 0, 30, 30)
	// These take the kernel's free-space-only branch, before it reads any
	// heights. Long hilly profiles cannot exercise a zero-, one- or two-point
	// profile, even when they happen to have no diffraction.
	for _, hs := range [][]float32{nil, {100}, {100, 130}} {
		p.Add(hs, 8000, 10, 1.5)
	}
	return p
}

// The pairs kernel and its CPU twin have to agree (ADR-0004).
func comparePairLossCPU(t *testing.T) {
	d := comparisonDevice(t)

	p := syntheticProfiles(600, 9001)
	got, err := d.PairProfileLoss(p, 869.618)
	if err != nil {
		t.Fatalf("pair loss on the GPU: %v", err)
	}
	want := propagation.ProfilePairLossCPU(p, 869.618)

	// The profile heights are shared byte for byte, so only f32
	// transcendentals differ: hold the twins within a tenth of a dB.
	worst, err := lossDifference(got, want, 0.1)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d pairs compared, worst disagreement %.4f dB", len(want), worst)
}

// What the GPU is worth at the size that made this necessary.
func TestPairLossIsWorthIt(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}
	d := comparisonDevice(t)

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

	worst, err := lossDifference(got, want, 0.1)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d pairs: GPU %v, one core %v (%.1fx), worst disagreement %.4f dB",
		len(want), onGPU.Round(time.Millisecond), onCPU.Round(time.Millisecond),
		float64(onCPU)/float64(onGPU), worst)
}
