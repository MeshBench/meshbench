// What a link costs and what a node spends: the link budget for a selection,
// and the solar and battery study for one node or the selection.
package session

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerBudgetVerbs(st *state.Store, s *Sim) {
	st.Handle("budget.for_selection", func(w *state.World, _ any) (any, error) {
		at := -1
		for i := range w.Nodes {
			if w.Nodes[i].Selected {
				at = i
				break
			}
		}
		w.Budgets = s.budgetsFor(at, w.Links)
		// The cut-through follows the budget: same pair, same numbers, so the
		// Link panel's picture and its margins cannot disagree.
		if len(w.Budgets) == 2 {
			s.profileFor(st, w.Budgets[0].From, w.Budgets[0].To,
				w.Budgets[0].MarginDB, w.Budgets[1].MarginDB)
		} else {
			w.LinkProfile = nil
		}
		return map[string]any{"budgets": len(w.Budgets)}, nil
	})

	// node.energy is energy.for_selection with the node named, for the node
	// window's own December button.
	st.Handle("node.energy", func(w *state.World, p any) (any, error) {
		name := soleString(p)
		at := -1
		for i := range w.Nodes {
			w.Nodes[i].Selected = w.Nodes[i].Name == name
			if w.Nodes[i].Selected {
				at = i
			}
		}
		if at < 0 {
			return nil, fmt.Errorf("no node named %q", name)
		}
		duty := 0.0
		for _, v := range w.Scores {
			if v.Name == name {
				duty = v.DutyCyclePct
			}
		}
		e, err := EnergyFor(s.nodes[at], duty)
		if err != nil {
			return nil, err
		}
		w.Energy = e
		w.Say(fmt.Sprintf("%s: worst state of charge %.0f%% on day %d, %d dead days",
			e.Node, e.WorstSoC*100, e.WorstDay, e.DeadDays))
		return map[string]any{"node": e.Node}, nil
	})

	st.Handle("energy.for_selection", func(w *state.World, _ any) (any, error) {
		at := -1
		for i := range w.Nodes {
			if w.Nodes[i].Selected {
				at = i
				break
			}
		}
		if at < 0 || at >= len(s.nodes) {
			return nil, fmt.Errorf("no node selected")
		}
		// The duty cycle the run measured, not one typed into a form.
		duty := 0.0
		for _, v := range w.Scores {
			if v.Name == w.Nodes[at].Name {
				duty = v.DutyCyclePct
			}
		}
		e, err := EnergyFor(s.nodes[at], duty)
		if err != nil {
			return nil, err
		}
		w.Energy = e
		w.Say(fmt.Sprintf("%s: worst state of charge %.0f%% on day %d, %d dead days",
			e.Node, e.WorstSoC*100, e.WorstDay, e.DeadDays))
		return map[string]any{"node": e.Node}, nil
	})
}
