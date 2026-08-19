package engine

import "sync/atomic"

// How many of a run's receptions turned on a decibel or two at the receiver.
//
// The 1.17.0 against 1.17.1 sweep asked whether a receive-gain change moved a
// mesh, and answered with delivery totals that could not have shown it: a flood
// that loses an edge arrives by another, so an effect real at link level is
// summed away before anybody sees it. Repeats do not fix that. Nothing about
// running the pair again produces a number the aggregation did not destroy.
//
// Determinism makes a better instrument available, and it does not need a second
// run at all. Every reception already computes its own margin against the
// demodulator's floor, so the run can count - exactly, as it goes - how many
// decodes would have been lost had the receiver been a little worse, and how
// many misses would have decoded had it been a little better. That is the paired
// comparison the plan called for, evaluated analytically instead of by varying
// the receiver and hoping the mesh does not diverge. It cannot diverge: there is
// only one run.
//
// It is also the honest way to report a figure resting on
// RxBoostedGainImprovementDB, which is a placeholder. Counting by margin band
// says what one decibel is worth and what three are, and leaves the reader to
// apply whichever they believe.

// MarginEdgesDB are the bands a reception's margin is counted into, in decibels
// from the demodulator's floor. Cumulative: a decode 0.5 dB above the floor is
// counted in every band.
var MarginEdgesDB = []float64{1, 2, 3, 6, 10}

// Sensitivity is how close a run's receptions ran to the edge of decoding.
type Sensitivity struct {
	// Decoded is every reception the demodulator accepted. Missed is those that
	// arrived measurably and were not demodulated - a signal below the floor is
	// an absence rather than a near miss, and is not counted.
	Decoded, Missed int

	// LostIfWorseBy[i] is decodes whose margin was under MarginEdgesDB[i]: they
	// would have been lost had the receiver been that much less sensitive.
	// WonIfBetterBy[i] is the mirror - misses that were within that of decoding.
	LostIfWorseBy, WonIfBetterBy []int
}

// AtRisk is the share of a run's decodes that a loss of d dB would have cost,
// for d the band at index i. The number a receiver study is actually after.
func (s Sensitivity) AtRisk(i int) float64 {
	if s.Decoded == 0 || i >= len(s.LostIfWorseBy) {
		return 0
	}
	return float64(s.LostIfWorseBy[i]) / float64(s.Decoded)
}

// sensitivity accumulates on the delivery path, which runs per transmission per
// receiver on a country-sized mesh. Atomics rather than the engine lock: the
// counting must not become the reason a run is slow.
type sensitivity struct {
	decoded, missed atomic.Int64
	lost, won       [8]atomic.Int64
}

// note records one reception's margin. Positive decoded, negative did not.
func (s *sensitivity) note(marginDB float64, decoded bool) {
	if decoded {
		s.decoded.Add(1)
		for i, e := range MarginEdgesDB {
			if marginDB < e {
				s.lost[i].Add(1)
			}
		}
		return
	}
	s.missed.Add(1)
	for i, e := range MarginEdgesDB {
		if -marginDB < e {
			s.won[i].Add(1)
		}
	}
}

func (s *sensitivity) read() Sensitivity {
	out := Sensitivity{
		Decoded:       int(s.decoded.Load()),
		Missed:        int(s.missed.Load()),
		LostIfWorseBy: make([]int, len(MarginEdgesDB)),
		WonIfBetterBy: make([]int, len(MarginEdgesDB)),
	}
	for i := range MarginEdgesDB {
		out.LostIfWorseBy[i] = int(s.lost[i].Load())
		out.WonIfBetterBy[i] = int(s.won[i].Load())
	}
	return out
}

// Sensitivity is how close this run's receptions have run to the edge so far.
func (e *Engine) Sensitivity() Sensitivity { return e.sens.read() }

// ResetSensitivity zeroes the counters, so one cell of a sweep reports its own
// receptions rather than every cell before it.
func (e *Engine) ResetSensitivity() {
	e.sens.decoded.Store(0)
	e.sens.missed.Store(0)
	for i := range e.sens.lost {
		e.sens.lost[i].Store(0)
		e.sens.won[i].Store(0)
	}
}
