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

	"github.com/MeshBench/meshbench/internal/dsp"
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
	// PhaseStepRad rotates the signal by this much per sample: a carrier
	// frequency offset as baseband sees it. An oscillator's ppm error, a
	// Doppler shift, an adjacent channel - all of them are exactly this.
	PhaseStepRad float64
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
		// Sub-sample delay as a phase rotation. At 125 kHz one sample is
		// 2.4 km, so mesh delays are almost always fractional — and phase is
		// exactly how two arrivals decide whether they reinforce or cancel.
		// One rotation per transmission, folded into the amplitude: computing
		// a transcendental per sample was 50% of a waveform run's CPU.
		rot := cmplx.Exp(complex(0, -2*math.Pi*frac)) * complex(amp, 0)

		// The overlap of this transmission with the window, so the hot loop
		// carries no bounds branch.
		off := tx.StartSample + intDelay
		lo, hi := 0, len(tx.Samples)
		if off < 0 {
			lo = -off
		}
		if off+hi > windowSamples {
			hi = windowSamples - off
		}
		if tx.PhaseStepRad == 0 {
			// Decomposed by hand: Go's complex multiply is exactly
			// (ac-bd, ad+bc), reproduced term for term so the sum stays
			// bit-identical - but on scalar registers with the bounds
			// checks hoisted, which the complex form does not get.
			rr, ri := real(rot), imag(rot)
			dst := out[off+lo : off+hi]
			src := tx.Samples[lo:hi]
			for i := range src {
				sr, si := real(src[i]), imag(src[i])
				dst[i] += complex(sr*rr-si*ri, sr*ri+si*rr)
			}
			continue
		}
		// A frequency offset is a rotation that never stops. Advanced from
		// the transmission's own first sample so a window that catches the
		// middle of a frame sees the phase the frame actually has there.
		s, c := math.Sincos(tx.PhaseStepRad * float64(lo))
		w := rot * complex(c, s)
		sStep, cStep := math.Sincos(tx.PhaseStepRad)
		step := complex(cStep, sStep)
		wr, wi := real(w), imag(w)
		tr, ti := real(step), imag(step)
		dst := out[off+lo : off+hi]
		src := tx.Samples[lo:hi]
		for i := range src {
			sr, si := real(src[i]), imag(src[i])
			dst[i] += complex(sr*wr-si*wi, sr*wi+si*wr)
			wr, wi = wr*tr-wi*ti, wr*ti+wi*tr
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
