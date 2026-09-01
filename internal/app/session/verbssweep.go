// Saving a run and planning a route - the questions bigger than one tick that
// still live in core; the sweep itself is its own package now.
package session

import (
	"context"
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerSweepVerbs(st *state.Store, s *Sim) {
	st.HandleSpec("run.save", state.Spec{
		What: "file the counters as they stand as a named run, so two " +
			"configurations measured an hour apart can still be put side by side",
		Params: []state.Param{
			{Name: "name", Type: state.ParamString, Primary: true,
				What: "what to file it under; empty files it as \"run\", and the " +
					"time is appended either way so two saves of one name both " +
					"survive"},
		},
		Returns: []string{"path"},
		Answers: "It writes into the user's cache directory, not the working " +
			"directory, and takes its numbers from the renderer's snapshot " +
			"rather than the world, so what is recorded is what was on screen " +
			"when it was asked for.",
		Example: &state.Example{
			Params: "baseline", What: "keep the run to compare the next one against",
			Runnable: false,
		},
	}, func(w *state.World, p any) (any, error) {
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

	st.HandleSpec("plan.routes", state.Spec{
		What: "search for the paths a message could take between the first and " +
			"last selected nodes, on a worker, and answer with only the pair " +
			"it has gone off to search between",
		Returns: []string{"from", "to"},
		Answers: "It takes its ends from the selection rather than from " +
			"parameters, and refuses where fewer than two nodes are selected. " +
			"The routes themselves arrive later through an internal callback " +
			"and land on the world; nothing here waits for them, and a search " +
			"that fails says so in the status line rather than in this answer.",
		Example: &state.Example{
			Params: map[string]any{}, What: "why a message gets from one end to the other",
			Runnable: false,
		},
	}, func(w *state.World, _ any) (any, error) {
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

	st.HandleInternalSpec("plan.set", state.Spec{
		What: "take a finished route search onto the world, which is the only " +
			"place the paths become something the map can draw",
		Returns: []string{"routes"},
		Answers: "Anything that is not a []state.Route leaves the world holding " +
			"no routes rather than being refused, so a caller from outside the " +
			"process would silently erase them - which is why the socket is " +
			"not allowed to reach it.",
	}, func(w *state.World, p any) (any, error) {
		routes, _ := p.([]state.Route)
		w.Routes = routes
		w.Say(fmt.Sprintf("%d route(s) found", len(routes)))
		return map[string]any{"routes": len(routes)}, nil
	})

	st.HandleInternalSpec("plan.failed", state.Spec{
		What: "put a route search's error in the status line, which is the only " +
			"place it can go: the search runs on a worker, long after the verb " +
			"that started it answered",
		Answers: "Nothing. It exists to say something to the operator.",
	}, func(w *state.World, p any) (any, error) {
		msg := soleString(p)
		w.Say("planning: " + msg)
		return nil, nil
	})
}
