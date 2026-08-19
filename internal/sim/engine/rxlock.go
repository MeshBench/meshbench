// One demodulator per receiver.
//
// A LoRa receiver locks onto a preamble and is then occupied for that packet's
// whole airtime. Everything else arriving inside that window is interference,
// however strong it is and however cleanly it would have decoded on its own.
// Nothing in the per-transmission judgement could express that: it asks "could
// this receiver have decoded this packet", once per pair, and two packets that
// both cleared their threshold both came back yes. Receivers demodulating two
// colliding packets in the same millisecond is what that looked like from a
// companion, and it is why floods here never died the way real ones do -
// collisions are where a flood loses copies, and this simulator was not losing
// any.
//
// This is not a rule about outcomes, which the channel is forbidden from
// making. Which packet survives a collision still emerges: the loser is
// usually beaten by summed interference long before it reaches here, and
// capture effect decides the rest. The only thing asserted is that there is
// one demodulator, which is a fact about the hardware rather than a verdict
// about a packet.
package engine

import (
	"math"

	"github.com/MeshBench/meshbench/internal/rf/dsp"
)

// demodulatorHeldBy names the transmission that already had this receiver's
// demodulator when t arrived, or "" if the receiver was free to listen.
//
// Deliberately stateless. A lock held in a map would have to be taken in the
// order transmissions start and released in the order they end, but they are
// judged in the order they *finish* - so a short packet starting later would
// settle first and steal a demodulator that a longer, earlier one was already
// holding. Deriving the answer from the concurrency set instead gives the same
// result whatever order the batch runs in, which is also what keeps a seeded
// run reproducible.
func (e *Engine) demodulatorHeldBy(rx int, t transmission,
	concurrent []transmission, nodes []*Node, txPHY phy) string {

	for _, other := range concurrent {
		if other.packetID == t.packetID || other.from == rx {
			// A node cannot lock onto its own transmission, and being deaf
			// while transmitting is half duplex - a separate cause, reported
			// separately, because it has a different fix.
			continue
		}
		// Only a preamble that arrived first can have taken the demodulator.
		// Ties break on packet ID so the answer never depends on slice order.
		if other.startMs > t.startMs {
			continue
		}
		if other.startMs == t.startMs && other.packetID > t.packetID {
			continue
		}
		if other.endMs <= t.startMs || other.startMs >= t.endMs {
			continue
		}
		otherPHY := e.phyOf(nodes[other.from].Spec)
		if !otherPHY.sameChannel(txPHY) {
			// A receiver is not locked by a packet it is not tuned to hear,
			// which is the whole reason an operator splits a mesh across two
			// presets.
			continue
		}
		if !e.detectableAt(rx, other, nodes, otherPHY) {
			continue
		}
		return nodes[other.from].Spec.Name
	}
	return ""
}

// detectableAt reports whether a transmission arrived at this receiver loudly
// enough for its preamble to be found.
//
// The same threshold listen-before-talk uses, and for the same reason: a
// carrier strong enough to detect is one strong enough to occupy the receiver,
// whether or not its payload would have survived. A packet that locks the
// demodulator and then fails its CRC has still cost the receiver the airtime.
func (e *Engine) detectableAt(rx int, t transmission, nodes []*Node, txPHY phy) bool {
	loss, ok := e.pathLoss(t.from, rx)
	if !ok {
		return false
	}
	src, dst := nodes[t.from], nodes[rx]
	rxDBm := src.Spec.TxPowerDBm + gain(src.Spec) - loss + gain(dst.Spec)
	noiseDBm := dsp.NoiseFloorDBm(txPHY.bandwidthHz, e.noiseFigOf(dst.Spec))
	if extra := e.emitterNoiseAt(rx); !math.IsInf(extra, -1) {
		noiseDBm = addDBm(noiseDBm, extra)
	}
	return rxDBm-noiseDBm >= requiredSNRdB(txPHY.sf)
}

// busyDemodulatorDetail is the ledger's wording for the outcome, in one place
// so both physics modes say the same thing about the same cause.
func busyDemodulatorDetail(holder string) string {
	return "its demodulator was already locked to " + holder +
		"; a LoRa receiver decodes one packet at a time"
}
