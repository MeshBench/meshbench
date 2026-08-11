package dsp

import (
	"math"
	"testing"
)

// These are the figures the Technical Design page publishes and every
// sensitivity claim is measured against. Pinned deliberately: a drift here is
// invisible in a waterfall but moves every reported margin.
func TestNoiseFloorMatchesPublishedFigures(t *testing.T) {
	for _, tc := range []struct {
		bwHz, nf, want float64
	}{
		{125_000, 6, -117.0},
		{250_000, 6, -114.0},
		{500_000, 6, -111.0},
		{125_000, 0, -123.0}, // an ideal receiver, for reference
	} {
		got := NoiseFloorDBm(tc.bwHz, tc.nf)
		if math.Abs(got-tc.want) > 0.05 {
			t.Errorf("NoiseFloorDBm(%.0f, %.0f) = %.2f, want %.2f", tc.bwHz, tc.nf, got, tc.want)
		}
	}
}

func TestProcessingGain(t *testing.T) {
	// SF12 is the headline: ~36 dB of coherent gain is what lets LoRa decode
	// roughly 20 dB below the noise floor.
	for _, tc := range []struct {
		sf   int
		want float64
	}{
		{7, 21.0721}, {9, 27.0927}, {12, 36.1236},
	} {
		if got := ProcessingGainDB(tc.sf); math.Abs(got-tc.want) > 0.01 {
			t.Errorf("ProcessingGainDB(%d) = %.2f, want %.2f", tc.sf, got, tc.want)
		}
	}
}

func TestSymbolDuration(t *testing.T) {
	// SF7/125k = 1.024 ms, SF12/125k = 32.768 ms — a factor of exactly 32.
	sf7 := SymbolDuration(7, 125_000)
	sf12 := SymbolDuration(12, 125_000)
	if math.Abs(sf7-0.001024) > 1e-9 {
		t.Errorf("SF7 symbol = %v s, want 0.001024", sf7)
	}
	if math.Abs(sf12-0.032768) > 1e-9 {
		t.Errorf("SF12 symbol = %v s, want 0.032768", sf12)
	}
	if ratio := sf12 / sf7; math.Abs(ratio-32) > 1e-9 {
		t.Errorf("SF12/SF7 duration ratio = %v, want 32", ratio)
	}
}

func TestSamplesPerSymbol(t *testing.T) {
	if got := SamplesPerSymbol(7); got != 128 {
		t.Errorf("SF7 = %d samples, want 128", got)
	}
	if got := SamplesPerSymbol(12); got != 4096 {
		t.Errorf("SF12 = %d samples, want 4096", got)
	}
}
