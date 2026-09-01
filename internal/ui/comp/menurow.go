package comp

import (
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// This reuses MenuItem from the map's context menu: one shape for "an action a
// person can choose", wherever it is offered, so an entry can move between the
// two without being rewritten.

// MenuRow draws a row of actions for a named thing and returns the one chosen.
//
// A row rather than a floating popup: the node view is a table with space
// beneath it, and a popup over a table hides the row it is about - which is
// the row somebody is trying to read while deciding.
//
// One of these per panel, and so per window. The click state used to be a
// package-level map keyed by action, which every window in the process shared:
// the actions a node offers are the same strings for every node, so two
// pop-outs drawing their own menus wrote that map from two frame loops at
// once. A concurrent map write is fatal to the process rather than a panic
// something can recover, and it would take the run with it. Guarding the map
// would not have been enough either, because the two windows would still be
// handing input events to one Clickable and a press in one could be consumed
// by the other.
type MenuRow struct {
	// clicks are kept by action, so a menu rebuilt every frame does not lose
	// the press that was in flight.
	clicks map[string]*widget.Clickable
}

// Layout draws the row and reports the action chosen, or "" for none.
func (m *MenuRow) Layout(t *theme.Theme, gtx layout.Context, items []MenuItem, about string) string {
	if m.clicks == nil {
		m.clicks = map[string]*widget.Clickable{}
	}
	kids := []layout.FlexChild{layout.Rigid(
		Text(t, t.Sz.Caption, t.P.Dim, about+":  "))}
	chosen := ""
	for _, it := range items {
		it := it
		c, ok := m.clicks[it.Action]
		if !ok {
			c = &widget.Clickable{}
			m.clicks[it.Action] = c
		}
		if c.Clicked(gtx) {
			chosen = it.Action
		}
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				fg := t.P.Ink
				if c.Hovered() {
					fg = t.P.Accent
				}
				return layout.Inset{Left: t.Sp.S, Right: t.Sp.S,
					Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
					Text(t, t.Sz.Caption, fg, it.Label))
			})
		}))
	}
	layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
	return chosen
}
