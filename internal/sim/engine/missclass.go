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

// waveformMissClass is the cause for a miss the demodulator itself decided,
// and the phrase that names it.
//
// One thing is established without guessing: whether the wanted signal was
// under the demodulator's threshold on its own, which no quieter channel would
// have saved - a floor miss. Above that threshold there is a second thing the
// summed window does isolate: a concurrent same-channel signal within the
// capture margin. The wanted was loud enough on its own and a comparable or
// louder one was in the air with it, so it was lost to that - interference,
// which the calculated path names the same way. Above the floor with nothing
// else on the channel the chain failed for a reason it does not separate, and
// unclassified says so rather than naming the likeliest.
func waveformMissClass(c wfCandidate, r wfResult, sf int) (Class, string) {
	if estimatedSNRdB(c) < requiredSNRdB(sf) {
		return ClassFloor, ""
	}
	// A concurrent signal within the capture margin of the wanted - the same
	// threshold the calculated path and the demodulator lock use. Loud enough
	// on its own, lost to something at least as loud.
	if !math.IsInf(r.interfererDBm, -1) && r.interfererDBm >= c.rxDBm-captureThresholdDB {
		return ClassInterference, fmt.Sprintf(
			"lost to a concurrent signal at %.0f dBm against %.0f dBm wanted",
			dsp.ReportRSSIdBm(r.interfererDBm), dsp.ReportRSSIdBm(c.rxDBm))
	}
	return ClassUnclassified, ""
}

// estimatedSNRdB is the figure the class is decided on: what the gates worked
// out from received power and the noise floor, before the demodulator looked.
//
// Named because two different SNRs travel with a waveform miss and only one of
// them decides anything. The other is r.snrdB, what the demodulator measured
// off the samples, which saturates at the top of the reportable scale - so a
// miss classed "floor" on an estimate of -4 dB was printing "+15.0 dB measured
// SNR", and "too quiet on its own" beside a number at the top of the scale is
// a sentence that sends somebody to check their antenna for no reason.
func estimatedSNRdB(c wfCandidate) float64 { return c.rxDBm - c.noiseDBm }

// noTerrainDataClass is the cause for a path the terrain could not cover.
//
// Not a floor miss: nothing about the signal was measured, because the path
// was never evaluated. It is a gap in the data, and an operator who reads it
// as a weak link draws the wrong map.
const noTerrainDataClass = ClassUnclassified
