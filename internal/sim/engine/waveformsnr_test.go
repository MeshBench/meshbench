package engine

import (
	"math"
	"testing"
)

// Two different SNRs travel with a waveform miss and only one of them decides
// the class. The class is decided on the estimate the gates worked out from
// received power and the noise floor; the detail printed the figure the
// demodulator measured off the samples, which saturates at the top of the
// reportable scale. So 95 of 113 "floor" misses carried a positive number and
// 43 of them read "+15.0 dB" - and "too quiet on its own" beside a number at
// the top of the scale sends somebody to check an antenna that is fine.
func TestTheClassIsDecidedOnTheEstimate(t *testing.T) {
	const sf = 10
	need := requiredSNRdB(sf)

	// Well under the floor on the estimate: floor, whatever the demodulator
	// went on to measure.
	quiet := wfCandidate{rxDBm: -130, noiseDBm: -130 - (need - 4)}
	clear := wfResult{interfererDBm: math.Inf(-1)}
	if got, _ := waveformMissClass(quiet, clear, sf); got != ClassFloor {
		t.Errorf("an estimate %0.1f dB under the floor classed as %v, want %v",
			need-estimatedSNRdB(quiet), got, ClassFloor)
	}
	if est := estimatedSNRdB(quiet); est >= need {
		t.Errorf("the estimate came out %.1f dB, which is not under the %.1f dB floor", est, need)
	}

	// Loud enough on the estimate, and the channel was clear: not a floor
	// miss, and with nothing else in the air the cause is not isolated.
	loud := wfCandidate{rxDBm: -100, noiseDBm: -130}
	if got, _ := waveformMissClass(loud, clear, sf); got != ClassUnclassified {
		t.Errorf("a clear-channel miss above the floor classed as %v, want unclassified", got)
	}

	// Loud enough on its own, but a concurrent signal within the capture
	// margin was in the window: interference, the one thing the summed window
	// isolates. Waveform mode never named this before, so scenario 4's
	// expected collision verdict could not appear.
	col, cause := waveformMissClass(loud, wfResult{interfererDBm: -101}, sf)
	if col != ClassInterference {
		t.Errorf("a miss beside a %v-dBm interferer classed as %v, want interference", -101.0, col)
	}
	if cause == "" {
		t.Error("an interference miss carried no phrase naming the concurrent signal")
	}

	// A concurrent signal far below the wanted does not take it: still
	// unclassified, because the wanted was not lost to it.
	weakInt, _ := waveformMissClass(loud, wfResult{interfererDBm: -120}, sf)
	if weakInt != ClassUnclassified {
		t.Errorf("a miss beside a signal 20 dB down classed as %v, want unclassified", weakInt)
	}
}
