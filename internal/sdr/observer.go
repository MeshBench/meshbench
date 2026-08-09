// Package sdr is the receive-only observer: a node that demodulates nothing.
//
// An SDR observer sits in the scenario like any other node — a position, an
// antenna, a noise figure — but it has no firmware and no protocol. What it
// captures is the complex baseband that arrived at its antenna: every
// concurrent transmission summed coherently, plus noise, exactly as
// internal/rf produced it.
//
// That is the point. Everything else in the simulator eventually decides
// something — this packet decoded, that one collided. An observer decides
// nothing, so it is the honest check on all of it: if the waterfall does not
// show what the ledger claims happened, one of them is wrong.
package sdr

import (
	"fmt"
	"math"

	"github.com/A13xB0/meshcoresim/internal/dsp"
	"github.com/A13xB0/meshcoresim/internal/rf"
)

// Observer is a receive-only node.
type Observer struct {
	// Name identifies it in captures and exports.
	Name string

	// CentreHz is where it is tuned, and SampleRateHz is how much spectrum it
	// sees. See the note on Capture about what that currently means.
	CentreHz     float64
	SampleRateHz float64

	// NoiseFigureDB is the receiver's own noise contribution. A cheap RTL dongle
	// is around 6 dB; an Airspy with a preamp rather less. It sets the noise
	// floor the waterfall shows, so it is not cosmetic.
	NoiseFigureDB float64
}

// Capture is one observation window: raw IQ and the conditions that produced it.
type Capture struct {
	Observer Observer

	// IQ is the complex baseband, unprocessed. No filtering, no gain control, no
	// demodulation — what arrived.
	IQ []complex128

	// NoisePowerLinear is the thermal noise in this window, on the same scale as
	// a unit-amplitude signal. Kept so a spectrogram can be plotted against a
	// real floor rather than an arbitrary one.
	NoisePowerLinear float64
}

// Capture observes the channel.
//
// The transmissions must already be priced by the link budget for *this
// observer's* position and antenna, the same as for any receiver: an observer
// on a hill and one in a valley see different things, and that difference is
// the whole reason it is a placed node rather than a global probe.
//
// A real SDR at 2 Msps sees far more spectrum than one LoRa channel. Ours sees
// the simulated channel and nothing else, because the channel is what the
// engine generates — so SampleRateHz is the modelled bandwidth, and adjacent
// channels are absent rather than quiet. docs/shortcomings.md says so.
func (o Observer) Capture(txs []rf.Transmission, seed, offset uint64, windowSamples int) Capture {
	noise := linearNoisePower(o.SampleRateHz, o.NoiseFigureDB)

	iq := rf.Observe(txs, rf.Receiver{
		NoisePowerLinear: noise,
		Seed:             seed,
		Offset:           offset,
	}, windowSamples)

	return Capture{Observer: o, IQ: iq, NoisePowerLinear: noise}
}

// linearNoisePower converts the thermal noise floor to the channel's scale.
//
// The channel works in unit-amplitude terms: a transmission arrives with
// GainDB applied to an amplitude of 1, so "0 dB" is a reference carrier. The
// noise power on that scale is therefore the noise floor relative to that same
// reference, which the scenario fixes by choosing a reference level. Until
// scenarios carry one (MSIM-18), the reference is 0 dBm.
func linearNoisePower(bandwidthHz, noiseFigureDB float64) float64 {
	const referenceDBm = 0.0
	floorDBm := dsp.NoiseFloorDBm(bandwidthHz, noiseFigureDB)
	return dbToLinearPower(floorDBm - referenceDBm)
}

func dbToLinearPower(db float64) float64 {
	return math.Pow(10, db/10)
}

// String makes an observer legible in logs and in the ledger.
func (o Observer) String() string {
	return fmt.Sprintf("%s @ %.3f MHz, %.0f kHz, NF %.1f dB",
		o.Name, o.CentreHz/1e6, o.SampleRateHz/1e3, o.NoiseFigureDB)
}
