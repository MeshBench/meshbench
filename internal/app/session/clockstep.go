package session

import (
	"context"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// stepTick advances the engine by one tick's worth of simulated time.
//
// A tick's worth, not one engine step. The engine's step is fixed at its own
// 10 ms, and the tick used to take exactly one of them whatever the tick size
// said - then set the clock back from the engine - so sim.faster, sim.slower
// and sim.speed changed the number on the transport and nothing else: every
// run in a session went at real time, with the processor mostly idle.
//
// So the tick is owed its size and pays it in whole engine steps. A tick
// shorter than a step pays every few ticks, which is slower than real time; a
// tick longer than the machine can cover in its 10 ms of wall time simply runs
// as fast as the machine can go, which is what the lockstep clock exists to
// allow: firmware reads simulated time, so nothing in it notices.
//
// Due sends are checked after every step rather than once a tick, so a send at
// 20 s still goes at 20 s and not at the end of a 500 ms tick. The tick stops at
// the end of a timed run, so sim.run lands where it was asked to.
//
// And it stops at tickWallBudget of wall time. This runs on the store's
// goroutine, so a 1000 ms tick on a big native mesh - a hundred steps - held
// every verb and the interface for as long as those took. What is left is
// dropped rather than carried: a machine that cannot cover the tick runs as
// fast as it can, not further and further behind.
func (s *Sim) stepTick(w *state.World, tickMs uint32) {
	// Close takes the engine away, from the workbench's own shutdown
	// goroutine, while this tick stays installed - so a step arriving after a
	// close finds nothing there and the panic takes the store's goroutine
	// with it.
	eng := s.eng
	if eng == nil {
		return
	}
	step := eng.Config.StepMs
	if step == 0 {
		step = 10
	}
	s.tickOwed += tickMs
	began := time.Now()
	for s.tickOwed >= step {
		if until := w.RunUntilMs; until != 0 && eng.NowMs() >= until {
			s.tickOwed = 0
			return
		}
		_ = eng.Step(context.Background())
		s.tickOwed -= step
		// After the step, so a send lands on a mesh whose clock has already
		// reached its moment.
		s.fireDueSends(w)
		if time.Since(began) > tickWallBudget {
			s.tickOwed = 0
			return
		}
	}
}

// tickWallBudget is how long one tick may hold the store's goroutine. A
// variable so a test can make every step overrun it.
var tickWallBudget = 50 * time.Millisecond
