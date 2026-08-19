// The radio arithmetic: what a node transmits with, what it can hear, and
// whether two of them are even on the same channel.
//
// Small, dull functions that everything else leans on. They are together
// because they are the layer where a decibel is still a decibel - no packets,
// no scheduling, nothing that knows a mesh exists.
package engine

import (
	"math"

	"github.com/MeshBench/meshbench/internal/dsp"
	"github.com/MeshBench/meshbench/internal/packet"
	"github.com/MeshBench/meshbench/internal/scenario"
)

func gain(n scenario.Node) float64 {
	if n.Antenna.Pattern == nil {
		return 0
	}
	return n.Antenna.Pattern.PeakDBi() - n.Antenna.FeedlineDB
}

// addDBm sums two powers expressed in dBm. Adding decibels instead is a mistake
// that produces a plausible number and is wrong by tens of dB.
func addDBm(a, b float64) float64 {
	if math.IsInf(a, -1) {
		return b
	}
	if math.IsInf(b, -1) {
		return a
	}
	return 10 * math.Log10(math.Pow(10, a/10)+math.Pow(10, b/10))
}

// phy is one node's modem settings, resolved.
type phy struct {
	freqMHz     float64
	bandwidthHz float64
	sf          int
	codingRate  int
}

// sameChannel reports whether two radios can hear each other at all.
//
// Frequency and bandwidth must match; spreading factor too, because LoRa's
// orthogonality across SFs is the property the whole modulation is sold on — a
// receiver at SF10 does not demodulate SF7, it sees noise. Coding rate is in
// the explicit header, so it does not have to match.
func (a phy) sameChannel(b phy) bool {
	return math.Abs(a.freqMHz-b.freqMHz) < 0.001 &&
		a.bandwidthHz == b.bandwidthHz && a.sf == b.sf
}

// noiseFigOf resolves a node's receive noise figure, falling back to the
// scenario's default for nodes imported without one.
//
// Per node rather than per run because a repeater with a masthead preamp and a
// handheld in a pocket do not have the same one. scenario.Node has carried the
// field since import and the engine simply never read it, so every node in
// every result so far has been given the run-wide figure regardless of what its
// board profile said.
func (e *Engine) noiseFigOf(n scenario.Node) float64 {
	if n.NoiseFigureDB > 0 {
		return n.NoiseFigureDB
	}
	return e.Config.NoiseFigDB
}

// phyOf resolves a node's radio, falling back to the scenario's defaults for
// nodes imported without one.
func (e *Engine) phyOf(n scenario.Node) phy {
	p := phy{
		freqMHz:     n.Radio.CentreHz / 1e6,
		bandwidthHz: n.Radio.BandwidthHz,
		sf:          n.Radio.SpreadFactor,
		codingRate:  n.Radio.CodingRate,
	}
	if p.freqMHz <= 0 {
		p.freqMHz = e.Config.FreqMHz
	}
	if p.bandwidthHz <= 0 {
		p.bandwidthHz = e.Config.BandwidthHz
	}
	if p.sf <= 0 {
		p.sf = e.Config.SF
	}
	if p.codingRate <= 0 {
		p.codingRate = e.Config.CodingRate
	}
	return p
}

// requiredSNRdB is Semtech's published demodulator floor, which the modem has
// been measured against to within 1.6 dB (docs/shortcomings.md).
func requiredSNRdB(sf int) float64 {
	if v, ok := dsp.RequiredSNRdB[sf]; ok {
		return v
	}
	return -20
}

// payloadID identifies a message by its content, across every hop it takes.
//
// The header and the payload; deliberately *not* the path. A flood packet grows
// a hop hash at every relay, so hashing the whole frame gave the same message a
// new identity at each hop — every relay looked like a brand new message, no
// message could be followed across the mesh, and the unique-versus-redundant
// count was measuring nothing.
//
// The route bits are masked out of the header for the same reason: a node may
// re-route a packet it forwards, and a message that changed identity when it
// switched from flood to direct would break at exactly the interesting moment.
func payloadID(frame []byte) uint64 {
	d := packet.Dissect(frame)
	h := uint64(14695981039346656037)
	mix := func(b byte) {
		h ^= uint64(b)
		h *= 1099511628211
	}
	if d.Truncated {
		// Unparseable: fall back to the whole frame. It cannot be followed, but
		// two identical malformed frames should still be one thing.
		for _, b := range frame {
			mix(b)
		}
		return h
	}
	// Payload type only, not the route bits or the version.
	mix(d.PayloadType)
	for _, b := range d.Payload {
		mix(b)
	}
	return h
}

// PayloadIDForTest exposes the message identity for tests.
func PayloadIDForTest(frame []byte) uint64 { return payloadID(frame) }
