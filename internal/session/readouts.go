// The per-tick readouts, off the per-tick path.
//
// Everything here re-describes the run for the interface: the event tail,
// the per-node table, trails, scores. None of it steers the simulation, so
// none of it belongs on every engine step - the tick paces the engine's
// clock, and time spent here was simulated time not passing.
package session

import (
	"github.com/MeshBench/meshbench/internal/gui/state"
)

// readoutTail is how many of the most recent events the tables show.
const readoutTail = 2000

func (s *Sim) refreshReadouts(w *state.World, index map[string]int) {
	// Trails from the last few seconds of simulated time. Recomputed from
	// the event log rather than accumulated, so a seek backwards or a
	// rebuilt engine cannot leave a trail on the map for a transmission
	// that is no longer in the run.
	const trailWindowMs = 4000
	from := uint32(0)
	if w.NowMs > trailWindowMs {
		from = w.NowMs - trailWindowMs
	}
	w.Trails = s.trailsSince(from, index)
	s.refreshOpenPacket(w)
	s.refreshCompanions(w)
	w.Events, w.EventTotal = s.eventTail(readoutTail)
	w.Counts = s.eventCounts()
	w.Scores = s.scores()
	w.Stats = s.nodeStats(w.Events)
	if s.history == nil {
		s.history = newNodeHistory()
	}
	s.history.record(w.Stats)
	for i := range w.Nodes {
		if w.Nodes[i].Selected {
			w.Series = s.history.seriesFor(w.Nodes[i].Name)
			break
		}
	}
}
