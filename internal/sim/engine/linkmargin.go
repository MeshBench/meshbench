package engine

import (
	"math"
	"sort"

	"github.com/MeshBench/meshbench/internal/rf/dsp"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// How close a mesh's links sit to the edge of decoding, which is the property
// that decides whether a study of receiver sensitivity can measure anything at
// all on it.
//
// A change of a decibel or two at the receiver alters the outcome only on links
// already within a decibel or two of threshold. Everywhere else the packet was
// always going to arrive, or was never going to. So a mesh whose links all clear
// threshold by tens of decibels will return the same delivery counts whatever
// the receiver does, and a sweep that varies the receiver on it measures its own
// noise - which is what the 1.17.1 A/B did, at the cost of an hour a run.
//
// This is the same arithmetic Engine.deliver performs per transmission, over the
// link cache rather than over a packet in flight. It has to stay that way: a
// margin computed differently from the one the demodulator is judged against
// would answer a question nobody asked.

// LinkMargin is one direction of one link. Direction is the point - noise figure
// belongs to the receiver and transmit power to the transmitter, so A to B and B
// to A are different numbers, and CLAUDE.md requires both be presented.
type LinkMargin struct {
	From, To int
	// MarginDB is how far above its demodulator's floor the signal arrives.
	// Negative means it does not decode.
	MarginDB float64
}

// LinkMargins is the margin for every ordered pair the link cache has a path
// for, in no particular order.
//
// Pairs the cache has culled are absent rather than reported as unreachable: the
// cull removes paths whose loss makes them hopeless, and counting them as
// hopeless directions would bury the near-threshold population this exists to
// find under thousands of certainties.
func (e *Engine) LinkMargins() []LinkMargin {
	e.mu.Lock()
	nodes := make([]*Node, len(e.nodes))
	copy(nodes, e.nodes)
	e.mu.Unlock()

	out := make([]LinkMargin, 0, len(nodes)*8)
	for from, src := range nodes {
		txPHY := e.phyOf(src.specRef())
		for to, dst := range nodes {
			if from == to {
				continue
			}
			if !dst.specRef().Kind.RunsFirmware() && dst.specRef().Kind != scenario.SDRObserver {
				continue
			}
			if dst.specRef().Kind != scenario.SDRObserver && !txPHY.sameChannel(e.phyOf(dst.specRef())) {
				continue
			}
			loss, ok := e.pathLoss(from, to)
			if !ok {
				continue
			}
			noiseDBm := dsp.NoiseFloorDBm(txPHY.bandwidthHz, e.noiseFigOf(dst.specRef()))
			if extra := e.emitterNoiseAt(to); !math.IsInf(extra, -1) {
				noiseDBm = addDBm(noiseDBm, extra)
			}
			rxDBm := e.rxPowerDBm(nodes, from, to, loss)
			out = append(out, LinkMargin{
				From: from, To: to,
				MarginDB: rxDBm - noiseDBm - requiredSNRdB(txPHY.sf),
			})
		}
	}
	return out
}

// MarginSpread is what a scenario can and cannot be asked.
type MarginSpread struct {
	// Directions is every ordered pair with a path, decoding or not.
	Directions int
	// Decoding is how many of them arrive above their demodulator's floor.
	Decoding int
	// Median and P10 are over the decoding directions only. A distribution that
	// includes the hopeless ones has a median describing terrain, not margin.
	MedianDB, P10DB float64
	// Sensitive is how many decoding directions sit within SensitiveDB of the
	// floor - the ones a change at the receiver could actually flip.
	Sensitive   int
	SensitiveDB float64
}

// Fraction is the share of decoding directions close enough to threshold to be
// flipped by a change of SensitiveDB at the receiver.
//
// This is the number that decides whether a receiver study is worth running on
// a scenario. Near zero, no aggregate delivery metric can resolve the effect,
// however many repeats are spent on it.
func (s MarginSpread) Fraction() float64 {
	if s.Decoding == 0 {
		return 0
	}
	return float64(s.Sensitive) / float64(s.Decoding)
}

// Spread summarises margins for a change of d dB at the receiver.
func Spread(ms []LinkMargin, d float64) MarginSpread {
	s := MarginSpread{Directions: len(ms), SensitiveDB: d}
	ok := make([]float64, 0, len(ms))
	for _, m := range ms {
		if m.MarginDB < 0 {
			continue
		}
		ok = append(ok, m.MarginDB)
		if m.MarginDB < d {
			s.Sensitive++
		}
	}
	s.Decoding = len(ok)
	if len(ok) == 0 {
		return s
	}
	sort.Float64s(ok)
	s.MedianDB = ok[len(ok)/2]
	s.P10DB = ok[len(ok)/10]
	return s
}
