// The sweep verbs: run an offered-load sweep over the selected node, and take
// its result back into the world.
package sweep

import (
	"context"
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerSweep(st *state.Store, s *session.Sim) {
	st.HandleSpec("sweep.run", state.Spec{
		What: "push a rising offered load through one node until the network " +
			"stops carrying what it is given, which is the point a delivery " +
			"figure taken at one load cannot show",
		Returns: []string{"arms", "seeds"},
		Answers: "The shape of the plan it has just started, not a result. The " +
			"plan is fixed and takes no parameters: four message rates from " +
			"one every two seconds down to one every 250 ms, over six seeds. " +
			"Those two dozen short simulations run on a worker and the matrix arrives " +
			"later through an internal callback, with progress under the job " +
			"id `sweep`. The node swept is the first selected one, or the " +
			"scenario's first companion where nothing is selected. It refuses " +
			"where no engine has been built or no node is loaded.",
		Example: &state.Example{
			Params: map[string]any{}, What: "find where the selected node's mesh saturates",
			Runnable: false,
		},
	}, func(w *state.World, _ any) (any, error) {
		if s.Engine() == nil || len(s.Nodes()) == 0 {
			return nil, fmt.Errorf("no network to sweep")
		}
		node := FirstCompanion(s.Nodes())
		for i := range w.Nodes {
			if w.Nodes[i].Selected {
				node = w.Nodes[i].Name
				break
			}
		}
		plan := DefaultSweep(node)
		total := len(plan.Arms) * len(plan.Seeds)
		w.Say(fmt.Sprintf("sweeping %d arms over %d seeds from %s",
			len(plan.Arms), len(plan.Seeds), node))
		// On a worker, with progress: this is twenty-four short simulations
		// and the interface has to stay usable while they run.
		go func() {
			ctx := context.Background()
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: "sweep", What: "sweeping offered load", Total: total})
			m := runSweep(s, ctx, plan, func(done, of int) {
				_, _ = st.Do(ctx, "job.progress", state.Job{
					ID: "sweep", What: "sweeping offered load",
					Done: done, Total: of})
			})
			_, _ = st.Do(ctx, "sweep.set", m)
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: "sweep", What: "sweeping offered load",
				Done: total, Total: total, Finished: true})
		}()
		return map[string]any{"arms": len(plan.Arms), "seeds": len(plan.Seeds)}, nil
	})

	st.HandleInternalSpec("sweep.set", state.Spec{
		What: "take the finished offered-load matrix onto the world, on the one " +
			"goroutine allowed to apply it",
		Answers: "Nothing. It carries a *state.Matrix, which is a Go value " +
			"nothing outside the process can spell, so anything else is " +
			"refused rather than applied as an empty matrix.",
	}, func(w *state.World, p any) (any, error) {
		m, ok := p.(*state.Matrix)
		if !ok {
			return nil, session.WrongCallback("sweep.set")
		}
		w.Matrix = m
		if m != nil {
			w.Say(fmt.Sprintf("swept %d arms over %d seeds", len(m.Arms), len(m.Seeds)))
		}
		return nil, nil
	})
}

func init() { session.RegisterDomain(registerSweep) }
