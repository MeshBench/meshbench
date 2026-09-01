// Saving a run and planning a route - the questions bigger than one tick that
// still live in core; the sweep itself is its own package now.
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

	st.HandleInternal("plan.set", func(w *state.World, p any) (any, error) {
		routes, _ := p.([]state.Route)
		w.Routes = routes
		w.Say(fmt.Sprintf("%d route(s) found", len(routes)))
		return map[string]any{"routes": len(routes)}, nil
	})

	st.HandleInternal("plan.failed", func(w *state.World, p any) (any, error) {
		msg := soleString(p)
		w.Say("planning: " + msg)
		return nil, nil
	})
}
