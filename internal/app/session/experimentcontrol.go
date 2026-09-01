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
	st.HandleSpec("experiment.state", state.Spec{
		What: "ask the runner where it has got to without disturbing it, which " +
			"is what a script polls between starting a sweep and reading it",
		Returns: []string{"arms", "seeds", "senders", "runs", "run_for_ms",
			"send_at_ms", "spread_ms", "bytes", "scope", "arm_labels",
			"running", "done", "status", "log"},
		Answers: "Everything experiment.define answers with, plus `running`, " +
			"`done` as the number of cells finished, `status` naming the cell " +
			"in flight, and `log`, the last twelve lines, which is absent " +
			"until something has been logged.",
		Example: &state.Example{
			Params: map[string]any{}, What: "poll a sweep in progress", Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
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

	st.HandleSpec("experiment.start", state.Spec{
		What: "put every arm through every seed on a worker, each cell in its " +
			"own engine with node storage of its own, and answer as soon as " +
			"the worker is away rather than when it is done",
		Returns: []string{"running", "runs"},
		Answers: "`runs` is how many cells were queued. Poll experiment.state " +
			"until `running` goes false. It refuses where a sweep is already " +
			"running, where no network is loaded, where no sender has been " +
			"named, and where the last sweep's cell in flight has not yet let " +
			"go of the results table.",
		Example: &state.Example{
			Params: map[string]any{}, What: "start the matrix that is defined",
			Runnable: false,
		},
	}, func(w *state.World, _ any) (any, error) {
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
		e.mu.Unlock()

		w.Jobs = append(w.Jobs, state.Job{
			ID: "experiment", What: "running arms", Total: e.runsTotal()})
		go s.runExperiment(ctx, st, e, nodes)
		return map[string]any{"running": true, "runs": e.runsTotal()}, nil
	})

	st.HandleSpec("experiment.stop", state.Spec{
		What: "ask a sweep to stop and say whether it actually has, since the " +
			"cell in flight finishes before the worker leaves and waiting for " +
			"it here would deadlock the worker",
		Returns: []string{"stopped", "done", "total", "settled"},
		Answers: "`stopped` is whether there was anything running to stop, so " +
			"false is a normal answer. `settled` is the one a script wants: " +
			"false means the worker is still inside a cell, and the next " +
			"experiment.start will be refused until it is not.",
		Example: &state.Example{
			Params: map[string]any{}, What: "abandon a sweep part way through",
			Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
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
