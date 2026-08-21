package dsp

import "testing"

// The preamble detector commits below the packet-decode floor, and stops
// committing not far below it.
//
// Both halves matter, and they are the two things #125's blocking gate needs
// to know. A receiver's demodulator is taken by a preamble it can *detect*,
// whether or not the packet behind it will decode - so a gate built on
// RequiredSNRdB, which is a decode threshold, excludes signals that can take
// the lock. And a gate with no floor at all would let a carrier far under the
// noise hold a receiver, which is the other way to get it wrong.
//
// Measured here rather than asserted from a datasheet: detection is five stable
// dechirped windows (PreambleDetectSymbols), and what that costs in SNR is a
// property of this implementation.
func TestTheDetectorCommitsBelowTheDecodeFloor(t *testing.T) {
	const trials = 30
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

		// At the decode floor the detector must be certain: anything the
		// receiver could decode, it must first have been able to lock onto.
		if got := detect(sf, floor); got < trials {
			t.Errorf("SF%d: at the decode floor %.1f dB the preamble was detected "+
				"%d/%d times; a packet that decodes must first be acquired",
				sf, floor, got, trials)
		}

		// Three dB below it, most of the time. This is the band a gate built on
		// RequiredSNRdB wrongly excludes - signals that cannot be decoded but
		// can take the demodulator, which is what a lock is.
		if got := detect(sf, floor-3); got < trials/2 {
			t.Errorf("SF%d: 3 dB below the decode floor (%.1f dB) the preamble was "+
				"detected only %d/%d times; the lock is supposed to reach further "+
				"down than the decode does", sf, floor-3, got, trials)
		}

		// Far below, only the detector's own false alarms - it does not stop
		// dead, it degrades, and a gate has to sit above the noise it fires on.
		// A quarter of a trial run is generous; measured it is nearer a tenth.
		if got := detect(sf, floor-9); got > trials/4 {
			t.Errorf("SF%d: 9 dB below the decode floor (%.1f dB) the preamble was "+
				"detected %d/%d times; that far down should be false alarms, not "+
				"acquisition", sf, floor-9, got, trials)
		}
	}
}
