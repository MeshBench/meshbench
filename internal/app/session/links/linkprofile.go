package links

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerLinkProfile(st *state.Store, s *session.Sim) {
	st.HandleInternal("link.profile_set", func(w *state.World, p any) (any, error) {
		prof, ok := p.(*state.Profile)
		if !ok {
			return nil, session.WrongCallback("link.profile_set")
		}
		w.LinkProfile = prof
		if prof != nil {
			return map[string]any{"from": prof.From, "to": prof.To,
				"km": prof.DistanceKm, "edges": len(prof.Edges)}, nil
		}
		return nil, nil
	})

	// link.profile computes for the selected pair, for scripts and captures;
	// the interface reaches the same worker through budget.for_selection.
	st.Handle("link.profile", func(w *state.World, _ any) (any, error) {
		var sel []string
		for i := range w.Nodes {
			if w.Nodes[i].Selected {
				sel = append(sel, w.Nodes[i].Name)
			}
		}
		if len(sel) < 2 {
			return nil, fmt.Errorf("select two nodes to cut through between")
		}
		s.ProfileFor(st, sel[0], sel[len(sel)-1], 0, 0)
		return map[string]any{"from": sel[0], "to": sel[len(sel)-1]}, nil
	})
}
