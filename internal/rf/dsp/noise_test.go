package dsp

import (
	"math"
	"testing"
)

// The inverse-CDF generator must be a standard normal where it counts: the
// moments, and the tails the FEC threshold lives in.
func TestInverseCDFNoiseIsStandardNormal(t *testing.T) {
	p := Philox{Seed: 42}
	const n = 2_000_000
	var sum, sumSq float64
	tail3, tail4 := 0, 0
	for i := uint64(0); i < n/2; i++ {
		a, b := p.normalPair(i)
		for _, v := range []float64{a, b} {
			sum += v
			sumSq += v * v
			if math.Abs(v) > 3 {
				tail3++
			}
			if math.Abs(v) > 4 {
				tail4++
			}
		}
	}
	mean := sum / n
	variance := sumSq/n - mean*mean
	if math.Abs(mean) > 0.005 {
		t.Fatalf("mean %f, want ~0", mean)
	}
	if math.Abs(variance-1) > 0.01 {
		t.Fatalf("variance %f, want ~1", variance)
	}
	// P(|Z|>3) = 2.70e-3, P(|Z|>4) = 6.33e-5.
	f3 := float64(tail3) / n
	f4 := float64(tail4) / n
	if f3 < 2.2e-3 || f3 > 3.2e-3 {
		t.Fatalf("3-sigma tail fraction %.2e, want ~2.7e-3", f3)
	}
	if f4 < 3e-5 || f4 > 1e-4 {
		t.Fatalf("4-sigma tail fraction %.2e, want ~6.3e-5", f4)
	}
}

func BenchmarkNormalPair(b *testing.B) {
	p := Philox{Seed: 7}
	for i := 0; i < b.N; i++ {
		_, _ = p.normalPair(uint64(i))
	}
}
