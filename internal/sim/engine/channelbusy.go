// Whether the air is clear, as each node can tell.
//
// This is the input to MeshCore's listen-before-talk, and the reason the
// firmware defers instead of transmitting into somebody else's packet. Nothing
// but the engine can work it out: a node's own radio has no view of the
// channel, and a node cannot hear itself.
package engine

import (
	"github.com/MeshBench/meshbench/internal/rf/dsp"
)

// channelBusy answers, for every node, whether another station is on the air
// loudly enough to be detected here.
//
// This is the input to MeshCore's listen-before-talk, and it is the whole
// reason the firmware defers instead of transmitting into somebody else's
// packet. Nothing but the engine can work it out: the node's own radio has no
// view of the channel, and a node cannot hear itself.
//
// Detection, not decoding. A LoRa receiver locks onto a preamble several dB
// below the level at which it could demodulate the payload, so the threshold is
// the demodulator floor for the current spreading factor rather than anything
// stricter - a carrier a node can detect but not decode is exactly the case
// listen-before-talk exists for.
func (e *Engine) channelBusy(now uint32) []bool {
	// Snapshot under the lock, compute outside it. pathLoss takes the same
	// mutex, and Go's is not reentrant: holding it across that call deadlocks
	// the frame thread on the first tick with anything in the air, which
	// presents as the window going black.
	e.mu.Lock()
	nodes := make([]*Node, len(e.nodes))
	copy(nodes, e.nodes)
	air := make([]transmission, 0, len(e.inFlight))
	for _, t := range e.inFlight {
		if now >= t.startMs && now < t.endMs {
			air = append(air, t)
		}
	}
	e.mu.Unlock()

	busy := make([]bool, len(nodes))
	if len(air) == 0 {
		return busy
	}
	e.mu.Lock()
	mode := e.Config.rfMode()
	e.mu.Unlock()
	if mode == RFWaveform {
		return e.waveformBusy(now, nodes, air)
	}
	for i, dst := range nodes {
		if dst.Firmware == nil {
			continue
		}
		rxPHY := e.phyOf(dst.Spec)
		for _, t := range air {
			// A node is deaf to the channel while its own transmitter is keyed,
			// and it is not listening for itself in any case.
			if t.from == i {
				continue
			}
			src := nodes[t.from]
			// Activity on another channel is not activity this node can detect
			// - the rule delivery already uses. Without it a node would defer
			// to a mesh it is not part of.
			txPHY := e.phyOf(src.Spec)
			if !txPHY.sameChannel(rxPHY) {
				continue
			}
			loss, ok := e.pathLoss(t.from, i)
			if !ok {
				continue
			}
			rxDBm := src.Spec.TxPowerDBm + gain(src.Spec) - loss + gain(dst.Spec)
			noiseDBm := dsp.NoiseFloorDBm(txPHY.bandwidthHz, e.noiseFigOf(dst.Spec))
			if rxDBm-noiseDBm >= requiredSNRdB(txPHY.sf) {
				busy[i] = true
				break
			}
		}
	}
	return busy
}
