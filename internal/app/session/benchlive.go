package session

import (
	"sync"

	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// A bench run happens in front of the operator, not behind them.
//
// Each cell of a sweep builds its own engine, with its own node storage, because
// a node keeps its settings between runs exactly as hardware does and an arm
// sharing storage with the previous one never reaches the changed default. The
// cost of that isolation was that the workbench watched none of it: the clock
// stayed at zero, the map stayed still, and the only sign a sweep was running
// was a line of text. Somebody starting an hour-long run could not tell it from
// a hung one.
//
// So the cell's engine is published while it runs, and everything that draws the
// simulation reads whichever engine is live rather than the session's own. The
// isolation is untouched - it is still a separate engine with separate storage,
// and it is still thrown away at the end of the cell.

// benchLive holds the engine a running sweep cell owns, if any.
type benchLive struct {
	mu  sync.Mutex
	eng *engine.Engine
}

// take publishes a cell's engine as the live one. Passing nil hands the view
// back to the session's own engine, which is what happens when a cell ends.
func (b *benchLive) take(e *engine.Engine) {
	b.mu.Lock()
	b.eng = e
	b.mu.Unlock()
}

// get is the engine a sweep cell owns, or nil.
func (b *benchLive) get() *engine.Engine {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.eng
}

// liveEngine is the engine the views should read: a running sweep cell's if
// there is one, otherwise the session's own.
//
// Everything that draws - trails, events, scores, per-node stats, the clock -
// goes through this. Everything that *owns* the session's engine, notably
// opening a project and warming the link matrix, deliberately does not: those
// belong to the session whatever a sweep is doing.
func (s *Sim) liveEngine() *engine.Engine {
	if e := s.bench.get(); e != nil {
		return e
	}
	return s.eng
}

// benchOwnsTheClock reports that a sweep cell is stepping time.
//
// The store's tick would otherwise step the engine a second time. A cell paces
// its own engine against wall time and counts on that pacing, so a stray extra
// step is not a faster run - it is a run whose airtime accounting is wrong.
func (s *Sim) benchOwnsTheClock() bool { return s.bench.get() != nil }
