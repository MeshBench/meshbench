package comp

import (
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// This reuses MenuItem from the map's context menu: one shape for "an action a
// person can choose", wherever it is offered, so an entry can move between the
// two without being rewritten.

// clicks are kept by action, so a menu rebuilt every frame does not lose the
// press that was in flight.
var menuClicks = map[string]*widget.Clickable{}

// MenuRow draws a row of actions for a named thing and returns the one chosen.
//
// A row rather than a floating popup: the node view is a table with space
// beneath it, and a popup over a table hides the row it is about - which is
// the row somebody is trying to read while deciding.
func MenuRow(t *theme.Theme, gtx layout.Context, items []MenuItem, about string) string {
	kids := []layout.FlexChild{layout.Rigid(
		Text(t, t.Sz.Caption, t.P.Dim, about+":  "))}
	chosen := ""
	for _, it := range items {
		it := it
		c, ok := menuClicks[it.Action]
		if !ok {
			c = &widget.Clickable{}
			menuClicks[it.Action] = c
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
