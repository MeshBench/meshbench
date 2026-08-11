package engine

import (
	"github.com/A13xB0/meshcoresim/internal/dsp"
	"github.com/A13xB0/meshcoresim/internal/rf"
)

// captureThresholdDB is how much stronger one signal must be for a receiver to
// lock it and ignore the other.
//
// Six dB is the figure LoRa capture is usually quoted at, and it is the reason
// a flood works at all: without capture, every simultaneous relay would destroy
// every other one.
const captureThresholdDB = 6

// CaptureThresholdDB is that figure, for a UI that has to explain a verdict.
func CaptureThresholdDB() float64 { return captureThresholdDB }

// InFlightTransmissions renders what is on the air right now as baseband, as
// one receiver would see it.
//
// The engine works in frames because that is what the firmware exchanges; a
// waterfall needs samples. Rather than keep every transmission modulated for
// the whole run — hundreds of megabytes nobody looks at — the samples are
// synthesised on demand from the frames still in flight, using the same
// modulator the demodulator is tested against.
//
// So the picture is of the same signal the channel carried, arrived at through
// a second path. If the two ever disagreed, the waterfall would be showing a
// different simulation from the one producing the results.
func (e *Engine) InFlightTransmissions(rxIndex int) []rf.Transmission {
	e.mu.Lock()
	inFlight := append([]transmission(nil), e.inFlight...)
	nodes := make([]*Node, len(e.nodes))
	copy(nodes, e.nodes)
	e.mu.Unlock()

	if rxIndex < 0 || rxIndex >= len(nodes) {
		return nil
	}
	rxPHY := e.phyOf(nodes[rxIndex].Spec)

	var out []rf.Transmission
	for _, t := range inFlight {
		src := nodes[t.from]
		txPHY := e.phyOf(src.Spec)
		// Only what this receiver could actually see. A transmission on another
		// channel is not in this waterfall, for the same reason it is not in
		// this receiver's ledger.
		if !txPHY.sameChannel(rxPHY) {
			continue
		}
		loss, ok := e.pathLoss(t.from, rxIndex)
		if !ok {
			continue
		}
		mod := dsp.Modulator{SF: txPHY.sf}
		out = append(out, rf.Transmission{
			Node:    src.Spec.Name,
			Samples: mod.Modulate(symbolsFor(t.frame, txPHY.sf)),
			GainDB:  src.Spec.TxPowerDBm + gain(src.Spec) - loss + gain(nodes[rxIndex].Spec),
		})
	}
	return out
}

// symbolsFor turns a frame into LoRa symbols.
//
// A faithful preamble and a real interleaver are not needed for a waterfall —
// what a chirp looks like on a spectrogram is decided by the spreading factor
// and the symbol values, and the values only have to come from the frame rather
// than from a random number generator. A collision between two real frames
// looks like a collision between two real frames.
func symbolsFor(frame []byte, sf int) []int {
	n := 1 << sf
	// Eight upchirps of preamble, as MeshCore's radio sends.
	syms := make([]int, 0, len(frame)+8)
	for i := 0; i < 8; i++ {
		syms = append(syms, 0)
	}
	for _, b := range frame {
		syms = append(syms, int(b)%n)
	}
	return syms
}
