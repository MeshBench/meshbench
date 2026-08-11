package dsp

import (
	"math"
	"testing"
)

// A symbol must survive a clean round trip, or nothing downstream is meaningful.
func TestRoundTripNoiseless(t *testing.T) {
	for _, sf := range []int{7, 9, 12} {
		m, d := Modulator{SF: sf}, Demodulator{SF: sf}
		n := SamplesPerSymbol(sf)
		for _, s := range []int{0, 1, 2, n / 3, n / 2, n - 1} {
			got, _ := d.DemodulateSymbol(m.ModulateSymbol(s))
			if got != s {
				t.Errorf("SF%d: sent symbol %d, recovered %d", sf, s, got)
			}
		}
	}
}

func TestRoundTripSequence(t *testing.T) {
	m, d := Modulator{SF: 8}, Demodulator{SF: 8}
	want := []int{0, 17, 250, 3, 128, 255}
	got := d.Demodulate(m.Modulate(want))
	if len(got) != len(want) {
		t.Fatalf("got %d symbols, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("symbol %d: got %d want %d", i, got[i], want[i])
		}
	}
}

// Noise is reproducible from its seed and offset — a requirement, not a nicety.
func TestNoiseDeterminism(t *testing.T) {
	a := make([]complex128, 64)
	b := make([]complex128, 64)
	Philox{Seed: 4417}.AddAWGN(a, 1.0, 0)
	Philox{Seed: 4417}.AddAWGN(b, 1.0, 0)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("same seed produced different noise at %d", i)
		}
	}
	c := make([]complex128, 64)
	Philox{Seed: 4418}.AddAWGN(c, 1.0, 0)
	same := true
	for i := range a {
		if a[i] != c[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("different seeds produced identical noise")
	}
}

// The generated noise must actually have the power it claims, or every
// sensitivity figure derived from it is wrong by the same unnoticed factor.
func TestNoisePowerIsCorrect(t *testing.T) {
	const want = 0.25
	x := make([]complex128, 1<<16)
	Philox{Seed: 99}.AddAWGN(x, want, 0)
	got := SignalPower(x)
	if math.Abs(got-want)/want > 0.05 {
		t.Errorf("noise power = %.4f, want %.4f (±5%%)", got, want)
	}
}
