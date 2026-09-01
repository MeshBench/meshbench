// Which cause a miss is recorded under.
//
// A miss is only worth recording if it says why, and the why is a claim about
// physics that somebody acts on: too quiet asks for more power or a better
// antenna, a busy demodulator asks for less traffic or different timing, and
// the two point opposite ways. So the cause is decided here, beside the
// arithmetic that established it, and travels on the event - never inferred
// afterwards from how the sentence happened to start.
//
// The rule these functions keep: never name a cause the branch did not
// establish. An unclassified miss is a question; a wrongly confident one is a
// wrong answer somebody spends money on.
package engine

import (
	"fmt"
	"math"

	"github.com/MeshBench/meshbench/internal/rf/dsp"
)

// weakMissCause is the wording and the class for a packet that did not clear
// the demodulator's threshold: too quiet on its own, or loud enough on its own
// and taken by something louder.
//
// Words and class together, in one place, because they are one statement. Said
// separately they drift, and a reworded sentence silently moves what the cards
// count.
func weakMissCause(snr, effective, required, interferenceDBm float64, sf int) (string, Class) {
	if !math.IsInf(interferenceDBm, -1) && snr >= required {
		return fmt.Sprintf("would have decoded at %.1f dB, lost to a stronger interferer",
			dsp.ReportSNRdB(snr)), ClassInterference
	}
	// Under the threshold on its own, so no interferer had to be involved and
	// a quieter channel would not have saved it.
	return fmt.Sprintf("SNR %.1f dB against %.1f dB needed at SF%d",
		effective, required, sf), ClassFloor
}

// waveformMissClass is the cause for a miss the demodulator itself decided.
//
// Waveform mode reports what the receive chain did, not what beat it: a window
// that failed may have been buried in noise, in a collider, in fading, or in
// all three, and the chain does not separate them. One thing is established
// without guessing - whether the wanted signal was under the demodulator's
// threshold on its own, which no quieter channel would have saved. Above that
// threshold the cause was not isolated, and saying so is better than naming
// the likeliest one and being believed.
func waveformMissClass(c wfCandidate, sf int) Class {
	if c.rxDBm-c.noiseDBm < requiredSNRdB(sf) {
		return ClassFloor
	}
	return ClassUnclassified
}

// noTerrainDataClass is the cause for a path the terrain could not cover.
//
// Not a floor miss: nothing about the signal was measured, because the path
// was never evaluated. It is a gap in the data, and an operator who reads it
// as a weak link draws the wrong map.
const noTerrainDataClass = ClassUnclassified
