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
// The lock is not first-come-keeps-it. The detector spends a few preamble
// symbols deciding before it commits - dsp.PreambleDetectSymbols of stable
// dechirped windows - and inside that contest the dominant signal wins, which
// is what capture effect means; measurement studies on real chips put the
// same commit at about four symbols. Only after the contest closes does
// arrival order rule. And a lock ends when its packet does: a holder that
// falls silent early in a long MeshCore preamble frees the receiver in time
// to acquire what is left, exactly as our own Detect would on the samples.
//
// This is not a rule about outcomes, which the channel is forbidden from
// making. Which packet survives a collision still emerges: the loser is
// usually beaten by summed interference or corruption long before it reaches
// here, and capture decides the rest. The only thing asserted is that there
// is one demodulator with one commitment window, which is a fact about the
// hardware rather than a verdict about a packet.
package engine

import (
	"math"

	"github.com/MeshBench/meshbench/internal/rf/dsp"
)

// demodulatorHeldBy names the transmission that had this receiver's
// demodulator when t needed it, or "" if the receiver could listen to t.
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

	symbolMs := 1000 * math.Pow(2, float64(txPHY.sf)) / txPHY.bandwidthHz
	// The contest window: how long the detector listens before committing.
	lockMs := uint32(math.Ceil(float64(dsp.PreambleDetectSymbols) * symbolMs))
	// t's whole preamble, which is what a freed receiver still has to work
	// with. MeshCore's preambles are long - 32 symbols at SF8 - and that
	// length is precisely what makes mid-preamble recovery a real event.
	preambleMs := uint32(math.Ceil(float64(dsp.PreambleSymbols(txPHY.sf)) * symbolMs))

	tDBm, ok := e.rxPowerAt(t.from, rx, nodes)
	if !ok {
		return ""
	}
	// The strongest qualifier is named, not the first found: with three
	// packets in one window the miss is a miss either way, but a ledger that
	// exists to name causes should name the transmission actually being
	// followed, and slice order is not a cause.
	holder, holderDBm := "", math.Inf(-1)
	claim := func(name string, dBm float64) {
		if dBm > holderDBm {
			holder, holderDBm = name, dBm
		}
	}
	for _, other := range concurrent {
		if other.packetID == t.packetID || other.from == rx {
			// A node cannot lock onto its own transmission, and being deaf
			// while transmitting is half duplex - a separate cause, reported
			// separately, because it has a different fix.
			continue
		}
		if other.endMs <= t.startMs || other.startMs >= t.endMs {
			continue
		}
		if other.startMs > t.startMs+lockMs {
			// t's contest had closed: the receiver was already following t,
			// and a later arrival can corrupt it but not take the demodulator.
			continue
		}
		otherPHY := e.phyOf(nodes[other.from].Spec)
		if !otherPHY.sameChannel(txPHY) {
			// A receiver is not locked by a packet it is not tuned to hear,
			// which is the whole reason an operator splits a mesh across two
			// presets.
			continue
		}
		otherDBm, detectable := e.detectableAt(rx, other, nodes, otherPHY)
		if !detectable {
			continue
		}
		if other.startMs+lockMs < t.startMs {
			// The receiver committed to other before t's preamble began. It
			// is only free again if other falls silent with enough of t's
			// preamble left to acquire: the detector's run plus a symbol of
			// alignment slack, measured against the same preamble length the
			// airtime formula uses.
			if t.startMs+preambleMs >= other.endMs+lockMs+uint32(math.Ceil(symbolMs)) {
				continue
			}
			claim(nodes[other.from].Spec.Name, otherDBm)
			continue
		}
		// Both preambles inside one contest window: the dominant signal wins
		// the lock, which is capture effect at the moment it actually
		// happens. Ties fall to the earlier arrival, then to packet ID, so
		// the answer never depends on slice order.
		switch {
		case otherDBm > tDBm:
			claim(nodes[other.from].Spec.Name, otherDBm)
		case otherDBm < tDBm:
		case other.startMs < t.startMs:
			claim(nodes[other.from].Spec.Name, otherDBm)
		case other.startMs == t.startMs && other.packetID < t.packetID:
			claim(nodes[other.from].Spec.Name, otherDBm)
		}
	}
	return holder
}

// rxPowerAt is one transmitter's power at one receiver's antenna, through the
// same terrain every other judgement uses.
func (e *Engine) rxPowerAt(from, rx int, nodes []*Node) (float64, bool) {
	loss, ok := e.pathLoss(from, rx)
	if !ok {
		return 0, false
	}
	src, dst := nodes[from], nodes[rx]
	return src.Spec.TxPowerDBm + gain(src.Spec) - loss + gain(dst.Spec), true
}

// detectableAt reports whether a transmission arrived at this receiver loudly
// enough for its preamble to be found, and how loudly.
//
// The same threshold listen-before-talk uses, and for the same reason: a
// carrier strong enough to detect is one strong enough to occupy the receiver,
// whether or not its payload would have survived. A packet that locks the
// demodulator and then fails its CRC has still cost the receiver the airtime.
func (e *Engine) detectableAt(rx int, t transmission, nodes []*Node, txPHY phy) (float64, bool) {
	rxDBm, ok := e.rxPowerAt(t.from, rx, nodes)
	if !ok {
		return 0, false
	}
	noiseDBm := dsp.NoiseFloorDBm(txPHY.bandwidthHz, e.noiseFigOf(nodes[rx].Spec))
	if extra := e.emitterNoiseAt(rx); !math.IsInf(extra, -1) {
		noiseDBm = addDBm(noiseDBm, extra)
	}
	return rxDBm, rxDBm-noiseDBm >= requiredSNRdB(txPHY.sf)
}

// busyDemodulatorDetail is the ledger's wording for the outcome, in one place
// so both physics modes say the same thing about the same cause.
func busyDemodulatorDetail(holder string) string {
	return "its demodulator was already locked to " + holder +
		"; a LoRa receiver decodes one packet at a time"
}
