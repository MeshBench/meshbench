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
// The lock is resolved as a timeline, not a pairwise test. The first version
// asked "did anything detectable start before t and overlap it" - which let a
// packet that had itself LOST the demodulator phantom-hold it against every
// successor. Under a mesh's continuous traffic the phantoms chained: Z held
// off A, the dead A "held" off B, and a fixture went from thousands of
// receptions to fifty-three, with a companion that could not hear its own
// message repeated. What real hardware does - and this now models - is
// simpler: the demodulator is free until a detectable preamble arrives, the
// dominant arrival inside the detector's commitment window takes the lock,
// holds it for exactly that packet's airtime, and everything else in that
// span is lost. When it frees, the next acquirable preamble takes it, which
// is also what lets a receiver catch a packet whose predecessor ended early
// in a long MeshCore preamble.
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
	"sort"

	"github.com/MeshBench/meshbench/internal/rf/dsp"
)

// demodulatorHeldBy names the transmission actually holding this receiver's
// demodulator when t needed it, or "" if the receiver locked t itself.
//
// Deliberately stateless: the timeline is re-derived from the concurrency set
// on every ask, so the answer does not depend on the order a batch settles
// in, which is what keeps a seeded run reproducible.
func (e *Engine) demodulatorHeldBy(rx int, t transmission,
	concurrent []transmission, nodes []*Node, txPHY phy) string {

	symbolMs := 1000 * math.Pow(2, float64(txPHY.sf)) / txPHY.bandwidthHz
	// The contest window: how long the detector listens before committing.
	// Our own Detect needs this many stable symbols; chip measurement
	// literature puts the commit near four.
	lockMs := uint32(math.Ceil(float64(dsp.PreambleDetectSymbols) * symbolMs))
	preambleMs := uint32(math.Ceil(float64(dsp.PreambleSymbols(txPHY.sf)) * symbolMs))

	// The candidates: everything detectable at this receiver that overlaps
	// anything relevant, t included, in start order. The receiver's own
	// transmissions are not candidates - being deaf while keyed is half
	// duplex, a separate cause with a separate fix.
	type cand struct {
		start, end uint32
		dBm        float64
		id         uint64
		name       string
		isT        bool
	}
	var cands []cand
	for _, other := range concurrent {
		if other.from == rx {
			continue
		}
		if other.packetID == t.packetID {
			tDBm, ok := e.rxPowerAt(t.from, rx, nodes)
			if !ok {
				return ""
			}
			cands = append(cands, cand{t.startMs, t.endMs, tDBm, t.packetID,
				nodes[t.from].Spec().Name, true})
			continue
		}
		otherPHY := e.phyOf(nodes[other.from].Spec())
		if !otherPHY.sameChannel(txPHY) {
			// A receiver is not locked by a packet it is not tuned to hear,
			// which is the whole reason an operator splits a mesh across two
			// presets.
			continue
		}
		dBm, detectable := e.detectableAt(rx, other, nodes, otherPHY)
		if !detectable {
			continue
		}
		cands = append(cands, cand{other.startMs, other.endMs, dBm,
			other.packetID, nodes[other.from].Spec().Name, false})
	}
	if len(cands) == 0 {
		return ""
	}
	// Start order; ties by packet ID so the walk never depends on slice order.
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].start != cands[j].start {
			return cands[i].start < cands[j].start
		}
		return cands[i].id < cands[j].id
	})

	// Walk the demodulator through the traffic, one acquisition at a time.
	// At each round: every unconsumed candidate has a moment its preamble
	// could be acquired - its own start, or the instant the current hold
	// frees, provided enough preamble survives the wait (the detector's run
	// plus a symbol of alignment slack). The earliest such moment opens a
	// contest window; the dominant signal inside it takes the lock for its
	// whole airtime. A candidate whose preamble cannot outlive the hold is
	// lost - and holds nothing, which is the phantom this walk exists to
	// kill.
	slack := lockMs + uint32(math.Ceil(symbolMs))
	consumed := make([]bool, len(cands))
	freeAt := uint32(0)
	holder := ""
	for {
		// The earliest acquirable moment among the survivors.
		const never = ^uint32(0)
		minAt := never
		for k, c := range cands {
			if consumed[k] {
				continue
			}
			at := c.start
			if at < freeAt {
				if c.start+preambleMs >= freeAt+slack && c.end > freeAt {
					at = freeAt
				} else {
					if c.isT {
						return holder
					}
					consumed[k] = true
					continue
				}
			}
			if at < minAt {
				minAt = at
			}
		}
		if minAt == never {
			// t was consumed as lost above, or the set is exhausted with t
			// never contested - which cannot happen, since t is a candidate
			// until decided. Free air either way.
			return ""
		}
		// The contest: dominance among everything acquirable inside the
		// detector's window of that moment. Ties fall to the earlier start,
		// then to packet ID, so the answer never depends on slice order.
		best := -1
		for k, c := range cands {
			if consumed[k] {
				continue
			}
			at := c.start
			if at < freeAt {
				at = freeAt // survivors of the check above
			}
			if at > minAt+lockMs {
				continue
			}
			if best < 0 {
				best = k
				continue
			}
			b := cands[best]
			switch {
			case c.dBm > b.dBm:
				best = k
			case c.dBm < b.dBm:
			case c.start < b.start:
				best = k
			case c.start == b.start && c.id < b.id:
				best = k
			}
		}
		if cands[best].isT {
			return ""
		}
		holder = cands[best].name
		freeAt = cands[best].end
		consumed[best] = true
	}
}

// rxPowerAt is one transmitter's power at one receiver's antenna, through the
// same terrain every other judgement uses.
func (e *Engine) rxPowerAt(from, rx int, nodes []*Node) (float64, bool) {
	loss, ok := e.pathLoss(from, rx)
	if !ok {
		return 0, false
	}
	src, dst := nodes[from], nodes[rx]
	return src.Spec().TxPowerDBm + gain(src.Spec()) - loss + gain(dst.Spec()), true
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
	noiseDBm := dsp.NoiseFloorDBm(txPHY.bandwidthHz, e.noiseFigOf(nodes[rx].Spec()))
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
