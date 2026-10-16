package session

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A tick covers the tick's size in simulated time, not one engine step. It
// took one step whatever the speed control said, so "faster" relabelled the
// transport and the run went at real time regardless.
func TestATickCoversItsSize(t *testing.T) {
	s := &Sim{gpuAsked: true}
	s.build([]scenario.Node{repeaterNode("Alpha"), repeaterNode("Bravo")}, 869.618)
	w := &state.World{}
	before := s.eng.NowMs()
	s.stepTick(w, 250)
	if got := s.eng.NowMs() - before; got != 250 {
		t.Fatalf("a 250 ms tick advanced the engine %d ms", got)
	}
}

// A timed run ends where it was asked to, even when the tick is larger than
// what is left of it.
func TestATickStopsAtTheEndOfATimedRun(t *testing.T) {
	s := &Sim{gpuAsked: true}
	s.build([]scenario.Node{repeaterNode("Alpha"), repeaterNode("Bravo")}, 869.618)
	w := &state.World{RunUntilMs: s.eng.NowMs() + 120}
	s.stepTick(w, 1000)
	if s.eng.NowMs() != w.RunUntilMs {
		t.Fatalf("the run was to end at %d and the engine is at %d", w.RunUntilMs, s.eng.NowMs())
	}
	// And the next run does not inherit what the last tick was owed.
	w.RunUntilMs = 0
	before := s.eng.NowMs()
	s.stepTick(w, 10)
	if got := s.eng.NowMs() - before; got != 10 {
		t.Fatalf("a 10 ms tick after a timed run advanced the engine %d ms", got)
	}
}

// A tick shorter than an engine step steps only once it has been owed a whole
// one: a 5 ms tick is half speed, not real time.
func TestAShortTickStepsEveryOtherTime(t *testing.T) {
	s := &Sim{gpuAsked: true}
	s.build([]scenario.Node{repeaterNode("Alpha"), repeaterNode("Bravo")}, 869.618)
	w := &state.World{}
	before := s.eng.NowMs()
	for i := 0; i < 4; i++ {
		s.stepTick(w, 5)
	}
	if got := s.eng.NowMs() - before; got != 20 {
		t.Fatalf("four 5 ms ticks advanced the engine %d ms, want 20", got)
	}
}
