package dsp

import (
	"math"
	"testing"
)

// NormalAt is a single deterministic Gaussian draw keyed on the seed and a
// counter: the same seed and counter give the same value, different seeds give
// different values, and over many counters it is a standard normal.
func TestNormalAt(t *testing.T) {
	p := Philox{Seed: 4417}
	if p.NormalAt(42) != p.NormalAt(42) {
		t.Fatal("the same seed and counter did not reproduce")
	}
	if p.NormalAt(42) == (Philox{Seed: 99}).NormalAt(42) {
		t.Error("two seeds gave the same draw at one counter")
	}
	// Mean near zero and variance near one over a decent sample.
	const n = 20000
	var sum, sumsq float64
	for c := uint64(0); c < n; c++ {
		x := p.NormalAt(c)
		sum += x
		sumsq += x * x
	}
	mean := sum / n
	variance := sumsq/n - mean*mean
	if math.Abs(mean) > 0.05 {
		t.Errorf("mean %.3f is not near zero", mean)
	}
	if math.Abs(variance-1) > 0.1 {
		t.Errorf("variance %.3f is not near one", variance)
	}
}
