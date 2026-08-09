package dsp

import (
	"fmt"
	"math"
	"testing"
)

// The acceptance test for MSIM-2. Measures the SNR at which each spreading
// factor's symbol error rate crosses 1%, and compares against Semtech's
// published SX1262 figures.
func TestSensitivityAgainstSemtech(t *testing.T) {
	if testing.Short() {
		t.Skip("Monte Carlo sweep; run without -short")
	}
	fmt.Printf("\n  SF   measured   Semtech   delta   step\n")
	fmt.Printf("  ---------------------------------------\n")
	var prev float64
	var deltas []float64
	for _, sf := range []int{7, 8, 9, 10, 11, 12} {
		packets := 400
		if sf >= 11 {
			packets = 150 // 2^12 samples per symbol; keep the sweep tractable
		}
		// 1% PER over a 40-symbol frame — the quantity Semtech publishes.
		got := FindRequiredSNR(sf, 0.01, packets, 40, 4417)
		want := RequiredSNRdB[sf]
		step := 0.0
		if sf > 7 {
			step = got - prev
		}
		fmt.Printf("  SF%-2d  %7.2f   %7.2f  %+6.2f  %+5.2f\n", sf, got, want, got-want, step)
		prev = got
		deltas = append(deltas, got-want)

		// Each SF must land within 2 dB of the published figure.
		if math.Abs(got-want) > 2.0 {
			t.Errorf("SF%d: measured %.2f dB, Semtech %.2f dB — outside the 2 dB acceptance band", sf, got, want)
		}
	}
	// The 2.5 dB per-SF spacing falls straight out of the processing gain, so
	// it is the strongest single check that the chain is right.
	for i := 1; i < len(deltas); i++ {
		if math.Abs(deltas[i]-deltas[i-1]) > 1.5 {
			t.Errorf("per-SF spacing drifts: delta %d = %.2f, delta %d = %.2f", i-1, deltas[i-1], i, deltas[i])
		}
	}
}
