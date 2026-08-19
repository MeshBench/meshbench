// Saving a run, sweeping a parameter, and planning a route - the three things
// that ask the engine a question bigger than one tick.
package session

import (
	"context"
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerSweepVerbs(st *state.Store, s *Sim) {
	st.Handle("run.save", func(w *state.World, p any) (any, error) {
		name := soleString(p)
		if name == "" {
			name = "run"
		}
		// Saved from the snapshot rather than from the world, so what is
		// recorded is exactly what was on screen when somebody pressed save.
		path, err := SaveRun(name, st.Snapshot(), BuildOf(s))
		if err != nil {
			return nil, err
		}
		w.Say("saved " + path)
		return map[string]any{"path": path}, nil
	})

	st.Handle("sweep.run", func(w *state.World, _ any) (any, error) {
		if s.eng == nil || len(s.nodes) == 0 {
			return nil, fmt.Errorf("no network to sweep")
		}
		node := FirstCompanion(s.nodes)
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
			m := s.runSweep(ctx, plan, func(done, of int) {
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

	st.Handle("plan.routes", func(w *state.World, _ any) (any, error) {
		var picked []string
		for i := range w.Nodes {
			if w.Nodes[i].Selected {
				picked = append(picked, w.Nodes[i].Name)
			}
		}
		if len(picked) < 2 {
			return nil, fmt.Errorf("select two nodes to plan between")
		}
		from, to := picked[0], picked[len(picked)-1]
		w.Say("searching for routes between " + from + " and " + to)
		go func() {
			ctx := context.Background()
			routes, err := s.routesBetween(from, to)
			if err != nil {
				_, _ = st.Do(ctx, "plan.failed", err.Error())
				return
			}
			_, _ = st.Do(ctx, "plan.set", routes)
		}()
		return map[string]any{"from": from, "to": to}, nil
	})

	st.Handle("plan.set", func(w *state.World, p any) (any, error) {
		routes, _ := p.([]state.Route)
		w.Routes = routes
		w.Say(fmt.Sprintf("%d route(s) found", len(routes)))
		return map[string]any{"routes": len(routes)}, nil
	})

	st.Handle("plan.failed", func(w *state.World, p any) (any, error) {
		msg := soleString(p)
		w.Say("planning: " + msg)
		return nil, nil
	})
}
