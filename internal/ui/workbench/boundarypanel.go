// The Boundary panel: the study area, and what it contains.
package workbench

import (
	"fmt"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// boundaryPanel is the study area and what it contains.
type boundaryPanel struct {
	tb   comp.Table
	init bool
}

func (p *boundaryPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "area", Width: 260, Sortable: true},
			{Title: "rings", Width: 80, Right: true, Mono: true},
			{Title: "holes", Width: 80, Right: true, Mono: true},
			{Title: "points", Right: true, Mono: true},
		}
		p.init = true
	}
	if s == nil {
		return layout.Dimensions{}
	}
	if len(s.Areas) == 0 {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
			"no boundary in this network - accept one to bound a study"))
	}
	rows := make([]comp.Row, 0, len(s.Areas))
	for _, a := range s.Areas {
		pts := 0
		for _, r := range a.Rings {
			pts += len(r)
		}
		for _, h := range a.Holes {
			pts += len(h)
		}
		rows = append(rows, comp.Row{Key: a.Name, Cells: []string{
			a.Name, fmt.Sprintf("%d", len(a.Rings)),
			fmt.Sprintf("%d", len(a.Holes)), fmt.Sprintf("%d", pts),
		}})
	}
	p.tb.SetRows(rows)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, "study area")),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, fmt.Sprintf(
			"nodes within %g km outside the boundary are simulated too, "+
				"because a repeater just over the line is still heard", s.MarginKm))),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.tb.Layout(t, gtx, nil)
		}),
	)
}
