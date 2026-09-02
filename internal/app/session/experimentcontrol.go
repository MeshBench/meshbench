// Starting, stopping and reporting on a run.
//
// Apart from the matrix verbs because the three of them share one hard fact
// that the definition verbs do not: the run happens on a goroutine that reports
// back through this same store, so nothing here may wait on it. A stop that
// blocked until the worker had gone would deadlock the worker, and a start that
// went ahead while the worker was still there would take the results table out
// from under it. Both are answered by asking the run whether it has finished,
// and refusing rather than waiting.
package session

import (
	"context"
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

func registerExperimentControl(st *state.Store, s *Sim) {
	st.Handle("experiment.state", func(w *state.World, _ any) (any, error) {
		e := s.experiment()
		e.mu.Lock()
		defer e.mu.Unlock()
		out := e.describe()
		out["running"] = e.running
		out["done"] = len(e.results)
		out["status"] = e.status
		if n := len(e.log); n > 0 {
			out["log"] = e.log[max(0, n-12):]
		}
		return out, nil
	})

	st.Handle("experiment.start", func(w *state.World, _ any) (any, error) {
		e := s.experiment()
		e.mu.Lock()
		if e.running {
			e.mu.Unlock()
			return nil, fmt.Errorf("an experiment is already running")
		}
		if len(s.nodes) == 0 {
			e.mu.Unlock()
			return nil, fmt.Errorf("no network loaded")
		}
		if len(e.Senders) == 0 {
			e.mu.Unlock()
			return nil, fmt.Errorf("experiment.start needs at least one sender")
		}
		// Refused rather than started alongside the last run's tail. Waiting
		// here is not open to us, and starting anyway is what raced stale cells
		// into a new run's table.
		if e.done != nil {
			select {
			case <-e.done:
			default:
				e.mu.Unlock()
				return nil, fmt.Errorf(
					"the last experiment is still stopping - its current cell has " +
						"to finish before a new run can have the results table")
			}
		}
		e.running, e.results, e.status = true, nil, "starting"
		e.done = make(chan struct{})
		ctx, cancel := context.WithCancel(context.Background())
		e.cancel = cancel
		nodes := append([]scenario.Node(nil), s.nodes...)
		// Said here, at the start, and not only over the finished table. This
		// is the moment somebody commits an hour of machine time to a
		// comparison, and it is the last one at which knowing the arms will not
		// be comparable can save any of it.
		e.notReproducible = scenario.NotReproducible(nodes)
		why := e.notReproducible
		if why != "" {
			e.logf("not a comparison: %s", why)
		}
		e.mu.Unlock()

		if why != "" {
			w.Say("sweep started, but the arms will not be comparable: " + why)
		}
		w.Jobs = append(w.Jobs, state.Job{
			ID: "experiment", What: "running arms", Total: e.runsTotal()})
		go s.runExperiment(ctx, st, e, nodes)
		// Both keys always, as sim.state answers them: a script that has to
		// test for a key's presence before it can read the answer is a script
		// that will one day skip the test.
		return map[string]any{"running": true, "runs": e.runsTotal(),
			"reproducible": why == "", "not_reproducible_why": why}, nil
	})

	st.Handle("experiment.stop", func(w *state.World, _ any) (any, error) {
		e := s.experiment()
		e.mu.Lock()
		was := e.running
		if e.cancel != nil {
			e.cancel()
		}
		e.running = false
		done := len(e.results)
		// Whether the worker has actually left, which is not the same question
		// as whether it has been asked to.
		settled := e.done == nil
		if e.done != nil {
			select {
			case <-e.done:
				settled = true
			default:
			}
		}
		e.mu.Unlock()
		w.Jobs = finishJob(w.Jobs, "experiment")
		if was && !settled {
			w.Say("experiment stopping - the cell in flight finishes first")
		} else {
			w.Say("experiment stopped")
		}
		return map[string]any{"stopped": was, "done": done,
			"total": e.runsTotal(), "settled": settled}, nil
	})
}
