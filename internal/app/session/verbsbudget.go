// What a link costs and what a node spends: the link budget for a selection,
// and the solar and battery study for one node or the selection.
package session

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerBudgetVerbs(st *state.Store, s *Sim) {
	st.HandleSpec("budget.for_selection", state.Spec{
		What: "break the selected node's strongest measured link into the " +
			"decibels it is made of, both ways, and cut the terrain through the " +
			"same pair so the picture and the margins cannot tell different " +
			"stories",
		Returns: []string{"budgets"},
		Answers: "`budgets` is 2 when there was a link to break down and 0 " +
			"otherwise - nothing selected, no engine built, or no link measured " +
			"yet - which is a state rather than an error. The two budgets are " +
			"the two directions and their totals differ: each end's antenna " +
			"gain is evaluated on the bearing towards the other, so a beam is a " +
			"different antenna each way round. Both are best cases, with no " +
			"multipath, no body loss and no oscillator error in them. The " +
			"breakdown itself goes into the snapshot rather than into this " +
			"answer, and the cut-through arrives later still.",
		Example: &state.Example{
			Params:   map[string]any{},
			What:     "see what the selected node's best link is made of",
			Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
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

	st.HandleSpec("node.energy", state.Spec{
		What: "run a year of sun and battery at one named node, at the duty " +
			"cycle the run measured for it rather than one typed into a form, " +
			"for the node window that has a name and no selection to work from",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true, Primary: true,
				What: "the node to study; a name this network has not got, or " +
					"none at all, is refused - and note that the call selects " +
					"whatever it names and deselects everything else"},
		},
		Returns: []string{"node"},
		Answers: "Refused outright unless MESHBENCH_ENERGY is set: the solar " +
			"model is not trusted yet, and a plausible worst-day figure from an " +
			"untrusted model is worse than none. The year itself - worst state " +
			"of charge, the day it falls on, the dead days - goes into the " +
			"snapshot and onto the status line rather than into this answer. " +
			"With no run behind it the duty cycle is zero, which sizes the site " +
			"against a node that never transmits.",
		Example: &state.Example{
			Params: "West Lomond",
			What:   "ask whether one repeater's pack survives December",
		},
	}, func(w *state.World, p any) (any, error) {
		if !EnergyEnabled() {
			return nil, ErrEnergyDisabled
		}
		name := soleString(p)
		at := -1
		for i := range w.Nodes {
			w.Nodes[i].Selected = w.Nodes[i].Name == name
			if w.Nodes[i].Selected {
				at = i
			}
		}
		if at < 0 {
			return nil, noSuchNode(name)
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

	st.HandleSpec("energy.for_selection", state.Spec{
		What: "run the same year of sun and battery at whichever node is " +
			"selected, for the panel that has a selection and no name",
		Returns: []string{"node"},
		Answers: "Refused outright unless MESHBENCH_ENERGY is set, and refused " +
			"again with nothing selected. The year goes into the snapshot and " +
			"onto the status line rather than into this answer, and the duty " +
			"cycle it is run at is the one the run measured, which is zero " +
			"where no run has happened yet.",
		Example: &state.Example{
			Params: map[string]any{},
			What:   "size the selected site's panel and pack",
		},
	}, func(w *state.World, _ any) (any, error) {
		if !EnergyEnabled() {
			return nil, ErrEnergyDisabled
		}
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
