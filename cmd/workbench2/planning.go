// Planning: what would it take to connect these two (6.23).
//
// Through internal/planning, whose search returns the fewest *new* sites
// rather than the fewest hops. Existing infrastructure is free, and a five-hop
// route over four repeaters that already exist is a better answer than a
// three-hop route needing two new masts - which is the opposite of what a
// plain shortest-path search returns.
package main

import (
	"fmt"

	"gioui.org/layout"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// planPanel shows the routes between the two selected nodes.
type planPanel struct {
	tb   comp.Table
	init bool
	// OnRun asks the store to search.
	OnRun func()
}

func (p *planPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "route", Width: 80, Right: true, Mono: true},
			{Title: "new sites", Width: 96, Right: true, Mono: true, Sortable: true},
			{Title: "hops", Width: 70, Right: true, Mono: true},
			{Title: "longest hop", Width: 110, Right: true, Mono: true, Sortable: true},
			{Title: "through"},
		}
		p.tb.SortCol, p.init = 1, true
	}
	body := func(gtx layout.Context) layout.Dimensions {
		if s == nil || len(s.Routes) == 0 {
			return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
				"select two nodes, then search - existing repeaters are reused for free"))
		}
		rows := make([]comp.Row, 0, len(s.Routes))
		for i, r := range s.Routes {
			rows = append(rows, comp.Row{
				Key: fmt.Sprintf("%d", i),
				Cells: []string{
					fmt.Sprintf("%d", i+1),
					fmt.Sprintf("%d", r.NewSites),
					fmt.Sprintf("%d", r.Hops),
					fmt.Sprintf("%.1f km", r.LongestHopKm),
					r.Through,
				},
			})
		}
		p.tb.SetRows(rows)
		return p.tb.Layout(t, gtx, nil)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, "fewest new sites, not fewest hops")),
		layout.Flexed(1, body),
	)
}
