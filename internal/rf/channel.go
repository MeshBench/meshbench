// Package rf is the channel: it sums waveforms, applies gain and delay, and adds
// noise. It decides nothing.
//
// There is deliberately no code here that says "if two transmissions overlap,
// both fail". Capture effect, partial collisions and sensitivity must emerge
// from summed waveforms and the demodulator, or this is a packet model wearing a
// costume (CLAUDE.md, ADR-0003).
package rf

import (
	"math"
	"math/cmplx"

	"github.com/A13xB0/meshcoresim/internal/dsp"
)

// Transmission is one node's waveform arriving at one receiver, already priced
// by the link budget.
type Transmission struct {
	// Node identifies the transmitter, for the reception ledger.
	Node string
	// Samples is the transmitted complex baseband at unit amplitude.
	Samples []complex128
	// GainDB is the total link budget from this transmitter to this receiver:
	// TX power, feedlines, antenna gains in the true directions, path loss.
	// Negative for a loss, which is the usual case.
	GainDB float64
	// DelaySamples is propagation delay. Fractional delay carries phase, which
	// is what makes summation coherent rather than a fudge.
	DelaySamples float64
	// StartSample is when this transmission begins at the receiver, relative to
	// the observation window. Non-zero is what makes collisions partial.
	StartSample int
}

// Receiver observes the channel over a window of samples.
type Receiver struct {
	// NoisePowerLinear is the thermal noise power in the channel, on the same
	// scale as a unit-amplitude signal.
	NoisePowerLinear float64
	// Seed and Offset make the noise realisation reproducible.
	Seed   uint64
	Offset uint64
}

// Observe returns what actually arrived at the antenna over the window: every
// concurrent transmission summed coherently, plus noise.
func Observe(txs []Transmission, rx Receiver, windowSamples int) []complex128 {
	out := make([]complex128, windowSamples)

	for _, tx := range txs {
		amp := math.Pow(10, tx.GainDB/20) // dB is power; amplitude is the half-power
		intDelay := int(math.Floor(tx.DelaySamples))
		frac := tx.DelaySamples - float64(intDelay)

		for i, s := range tx.Samples {
			pos := tx.StartSample + i + intDelay
			if pos < 0 || pos >= windowSamples {
				continue
			}
			// Sub-sample delay as a phase rotation. At 125 kHz one sample is
			// 2.4 km, so mesh delays are almost always fractional — and phase is
			// exactly how two arrivals decide whether they reinforce or cancel.
			v := s * cmplx.Exp(complex(0, -2*math.Pi*frac))
			out[pos] += v * complex(amp, 0)
		}
	}

	if rx.NoisePowerLinear > 0 {
		dsp.Philox{Seed: rx.Seed}.AddAWGN(out, rx.NoisePowerLinear, rx.Offset)
	}
	return out
}

// SNRdB is the measured signal-to-noise ratio of an observation against a known
// noise power — what the ledger reports, and what the firmware's packetScore
// reads.
func SNRdB(observed []complex128, noisePowerLinear float64) float64 {
	if noisePowerLinear <= 0 {
		return math.Inf(1)
	}
	total := dsp.SignalPower(observed)
	signal := total - noisePowerLinear
	if signal <= 0 {
		return math.Inf(-1)
	}
	return 10 * math.Log10(signal/noisePowerLinear)
}
