package dsp

import "testing"

// The preamble detector commits below the packet-decode floor, and stops
// committing not far below it.
//
// Both halves matter, and they are the two things a blocking gate needs to
// know. A receiver's demodulator is taken by a preamble it can *detect*,
// whether or not the packet behind it will decode - so a gate built on
// RequiredSNRdB, which is a decode threshold, excludes signals that can take
// the lock. And a gate with no floor at all would let a carrier far under the
// noise hold a receiver, which is the other way to get it wrong.
//
// Measured here rather than asserted from a datasheet: detection is five stable
// dechirped windows (PreambleDetectSymbols), and what that costs in SNR is a
// property of this implementation.
//
// Four hundred trials because thirty cannot tell 98% from 100%, and the
// difference matters: at thirty this test asserted the detector is *certain*
// at the decode floor and passed, when it is 394 in 400 for SF8. Anybody who
// raised the trial count would have been told the demodulator had regressed.
// What was measured, and what the bounds below are drawn loosely around:
//
//	                  at the floor   3 dB below   9 dB below
//	SF8               394/400        211/400      0/400
//	SF12              400/400        271/400      0/400
func TestTheDetectorCommitsBelowTheDecodeFloor(t *testing.T) {
	const trials = 400
	detect := func(sf int, snrDB float64) int {
		a, b := StandardSync(sf)
		layout := FrameLayout{SF: sf, Preamble: 16, SyncA: a, SyncB: b}
		sigPower := SignalPower(Modulator{SF: sf}.ModulateSymbol(0))
		rng := Philox{Seed: 20260821}
		hit := 0
		for i := 0; i < trials; i++ {
			iq := layout.FrameSamples([]int{1, 2, 3, 4})
			rng.AddAWGN(iq, NoisePowerForSNR(sigPower, snrDB), uint64(i)*7919+1<<40)
			if _, ok := Detect(iq, layout); ok {
				hit++
			}
		}
		return hit
	}

	for _, sf := range []int{8, 12} {
		floor := RequiredSNRdB[sf]

		// At the decode floor the detector is all but certain: anything the
		// receiver could decode, it must nearly always have been able to lock
		// onto first. Not *always* - it is 394 in 400 for SF8 - so the bound is
		// where the claim actually holds rather than where it reads best.
		if got := detect(sf, floor); got < trials*19/20 {
			t.Errorf("SF%d: at the decode floor %.1f dB the preamble was detected "+
				"%d/%d times; a packet that decodes must first be acquired",
				sf, floor, got, trials)
		}

		// Three dB below it, about half the time - 53% at SF8 and 68% at SF12.
		// This is the band a gate built on RequiredSNRdB wrongly excludes:
		// signals that cannot be decoded but can still take the demodulator,
		// which is what a lock is. A third is the bound, comfortably under what
		// was measured and comfortably over the nothing seen nine below.
		if got := detect(sf, floor-3); got < trials/3 {
			t.Errorf("SF%d: 3 dB below the decode floor (%.1f dB) the preamble was "+
				"detected only %d/%d times; the lock is supposed to reach further "+
				"down than the decode does", sf, floor-3, got, trials)
		}

		// Far below, only the detector's own false alarms - it does not stop
		// dead, it degrades, and a gate has to sit above the noise it fires on.
		// A twentieth is generous: measured, it is none in four hundred.
		if got := detect(sf, floor-9); got > trials/20 {
			t.Errorf("SF%d: 9 dB below the decode floor (%.1f dB) the preamble was "+
				"detected %d/%d times; that far down should be false alarms, not "+
				"acquisition", sf, floor-9, got, trials)
		}
	}
}
