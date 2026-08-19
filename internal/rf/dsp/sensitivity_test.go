package dsp

import (
	"fmt"
	"math"
	"sync"
	"testing"
)

// The acceptance test for MSIM-2. Measures the SNR at which each spreading
// factor's symbol error rate crosses 1%, and compares against Semtech's
// published SX1262 figures.
func TestSensitivityAgainstSemtech(t *testing.T) {
	if testing.Short() {
		t.Skip("Monte Carlo sweep; run without -short")
	}

	sfs := []int{7, 8, 9, 10, 11, 12}
	measured := make([]float64, len(sfs))

	// One goroutine per spreading factor. They are wholly independent — separate
	// seeds, separate waveforms, no shared state beyond the read-only chirp
	// cache — and running them serially left eleven cores idle while SF12, by
	// far the most expensive, ran alone at the end.
	var wg sync.WaitGroup
	for i, sf := range sfs {
		wg.Add(1)
		go func(i, sf int) {
			defer wg.Done()
			packets := 400
			if sf >= 11 {
				packets = 150 // 2^12 samples per symbol; keep the sweep tractable
			}
			// 1% PER over a 40-symbol frame — the quantity Semtech publishes.
			measured[i] = FindRequiredSNR(sf, 0.01, packets, 40, 4417)
		}(i, sf)
	}
	wg.Wait()

	fmt.Printf("\n  SF   measured   Semtech   delta   step\n")
	fmt.Printf("  ---------------------------------------\n")
	var prev float64
	var deltas []float64
	for i, sf := range sfs {
		got, want := measured[i], RequiredSNRdB[sf]
		step := 0.0
		if i > 0 {
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

// Splitting the Monte Carlo across cores is only safe because every packet's
// noise and symbol values come from counters derived from the packet index. If
// that ever stops being true the results become order-dependent, which would
// look like ordinary Monte Carlo noise rather than a bug.
func TestPacketErrorRateIsDeterministic(t *testing.T) {
	const sf, snr, syms, packets = 9, -13.0, 20, 120
	first := PacketErrorRate(sf, snr, syms, packets, 4417)
	for i := 0; i < 4; i++ {
		if got := PacketErrorRate(sf, snr, syms, packets, 4417); got != first {
			t.Fatalf("run %d gave %.6f, first run gave %.6f — the split is order-dependent",
				i+2, got, first)
		}
	}
}

// Two seeds must give genuinely independent noise, not the same stream at a
// different offset. Tested on the noise directly rather than through a packet
// error rate: PER is a multiple of 1/packets, so two different realisations
// frequently land on the same value and the test would pass while the seeds
// were being ignored entirely.
func TestSeedsGiveIndependentNoise(t *testing.T) {
	const n = 64
	a := make([]complex128, n)
	b := make([]complex128, n)
	Philox{Seed: 4417}.AddAWGN(a, 1.0, 0)
	Philox{Seed: 99}.AddAWGN(b, 1.0, 0)

	same := 0
	for i := range a {
		if a[i] == b[i] {
			same++
		}
	}
	if same > 0 {
		t.Errorf("%d of %d samples identical across two seeds", same, n)
	}

	// And the shift that the old additive seeding produced: seed s at counter c
	// must not equal seed s' at counter c+(s-s').
	shifted := make([]complex128, n)
	Philox{Seed: 99}.AddAWGN(shifted, 1.0, 4417-99)
	same = 0
	for i := range a {
		if a[i] == shifted[i] {
			same++
		}
	}
	if same > n/8 {
		t.Errorf("%d of %d samples match a seed-shifted stream; the seed is an offset, "+
			"not a mix, so two seeds are not independent", same, n)
	}
}

// The estimator saturates, so the simulator must too.
//
// Measured rather than assumed: 1,992 receptions carrying SNR from the real
// ScotMesh network have a median of +5.0 dB, a 90th percentile of +13.0 dB and
// a hard wall at +15.0 dB with nothing above it. A model that reports +94 dB
// is not merely optimistic - it is saying something no field instrument can
// contradict, because none can express it.
func TestReportSNRSaysWhatAModemCouldSay(t *testing.T) {
	if got := ReportSNRdB(94.1); got != ReportableSNRCeilingDB {
		t.Fatalf("an impossible reading survived: %.1f dB", got)
	}
	if got := ReportSNRdB(ReportableSNRCeilingDB); got != ReportableSNRCeilingDB {
		t.Fatalf("the ceiling itself is reportable, got %.1f dB", got)
	}
	// Everything a real radio actually reports passes through untouched: the
	// whole measured distribution, floor included.
	for _, v := range []float64{-13.5, -5.5, 0, 5.0, 13.0, 14.9} {
		if got := ReportSNRdB(v); got != v {
			t.Fatalf("ReportSNRdB(%.1f) = %.1f; readings a real radio makes "+
				"must not be altered", v, got)
		}
	}
	// Deliberately no floor: below the demodulator's limit a packet is not
	// reported because it is not received, and clamping there would put a
	// floor under the failures RequiredSNRdB exists to judge.
	if got := ReportSNRdB(-34.2); got != -34.2 {
		t.Fatalf("a floor appeared: ReportSNRdB(-34.2) = %.1f", got)
	}
}
