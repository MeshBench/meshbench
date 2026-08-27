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
	st.Handle("sweep.run", func(w *state.World, _ any) (any, error) {
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

	st.Handle("sweep.set", func(w *state.World, p any) (any, error) {
		m, _ := p.(*state.Matrix)
		w.Matrix = m
		if m != nil {
			w.Say(fmt.Sprintf("swept %d arms over %d seeds", len(m.Arms), len(m.Seeds)))
		}
		return nil, nil
	})
}

func init() { session.RegisterDomain(registerSweep) }
