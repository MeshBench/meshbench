package engine

import "testing"

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
	if got := waveformMissClass(quiet, sf); got != ClassFloor {
		t.Errorf("an estimate %0.1f dB under the floor classed as %v, want %v",
			need-estimatedSNRdB(quiet), got, ClassFloor)
	}
	if est := estimatedSNRdB(quiet); est >= need {
		t.Errorf("the estimate came out %.1f dB, which is not under the %.1f dB floor", est, need)
	}

	// Loud enough on the estimate: not a floor miss, so whatever went wrong
	// was not the link being too quiet.
	loud := wfCandidate{rxDBm: -100, noiseDBm: -130}
	if got := waveformMissClass(loud, sf); got == ClassFloor {
		t.Errorf("an estimate of %.1f dB classed as %v", estimatedSNRdB(loud), got)
	}
}
