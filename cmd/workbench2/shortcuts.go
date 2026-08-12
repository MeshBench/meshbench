// The shortcut sheet (11.2), generated from the registry.
//
// Generated, not written. A hand-written sheet is a second list that drifts
// from the bindings, and the first time it does, the sheet is worse than
// nothing: somebody trusts it and presses a key that does something else.
package main

import (
	"gioui.org/layout"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/shell"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

type shortcutsPanel struct {
	sh   *shell.Shell
	tb   comp.Table
	init bool
}

func (p *shortcutsPanel) Draw(t *theme.Theme, gtx layout.Context, _ *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "keys", Width: 150, Mono: true, Sortable: true},
			{Title: "does", Width: 420},
			{Title: "action", Mono: true},
		}
		p.init = true
	}
	list := p.sh.Shortcuts()
	rows := make([]comp.Row, 0, len(list))
	for _, s := range list {
		rows = append(rows, comp.Row{
			Key:   s.Action,
			Cells: []string{s.String(), s.What, s.Action},
		})
	}
	p.tb.SetRows(rows)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, "keyboard")),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
			"the action column is the same name a menu entry and a script use")),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.tb.Layout(t, gtx, nil)
		}),
	)
}
