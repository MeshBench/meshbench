package links

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerLinkProfile(st *state.Store, s *session.Sim) {
	st.HandleInternalSpec("link.profile_set", state.Spec{
		What: "hold the finished cut-through for the panel to draw, or clear it " +
			"where the analysis could not run",
		Returns: []string{"from", "to", "km", "edges"},
		Answers: "Answers nothing at all when it is handed no profile, which is " +
			"how a failed analysis takes the old picture off the panel rather " +
			"than leaving one of the wrong pair there.",
	}, func(w *state.World, p any) (any, error) {
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

	st.HandleSpec("link.profile", state.Spec{
		What: "cut the terrain through whatever two nodes are selected, for a " +
			"script or a capture that has no panel to press, where the " +
			"interface reaches the same worker through budget.for_selection",
		Returns: []string{"from", "to"},
		Answers: "Refuses unless two nodes are selected, and takes the first " +
			"and the last of them when more are. It answers with the two ends " +
			"as soon as the worker starts; the profile lands later through the " +
			"internal `link.profile_set` and carries no margins at all - both " +
			"directions read 0 dB, because this draws the ground and " +
			"`budget.for_selection` is what prices the link.",
		Example: &state.Example{
			Params: map[string]any{},
			What:   "cut through between the two selected nodes",
		},
	}, func(w *state.World, _ any) (any, error) {
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
