// The Console panel: what one node has been doing.
//
// Per node rather than one merged log, because the question a console answers is
// always about a particular node, and a merged log answers it by making somebody
// filter.
package workbench

import (
	"fmt"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// consolePanel is what one node has been doing.
//
// Per node rather than one merged log, because the question a console answers
// is always about a particular node, and a merged log answers it by making
// somebody filter.
type consolePanel struct {
	tb   comp.Table
	init bool
}

func (p *consolePanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "at", Width: 76, Right: true, Mono: true, Sortable: true},
			{Title: "", Width: 46},
			{Title: "with", Width: 170},
			{Title: "detail"},
		}
		p.tb.SortCol, p.tb.SortDesc, p.init = 0, true, true
	}
	if s == nil {
		return layout.Dimensions{}
	}
	who := ""
	for i := range s.Nodes {
		if s.Nodes[i].Selected {
			who = s.Nodes[i].Name
			break
		}
	}
	if who == "" {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
			"select a node to see its console"))
	}
	rows := make([]comp.Row, 0, 64)
	for i := range s.Events {
		e := &s.Events[i]
		if e.From != who && e.To != who {
			continue
		}
		other := e.To
		if e.To == who {
			other = e.From
		}
		rows = append(rows, comp.Row{
			Key: fmt.Sprintf("%d/%d", e.PacketID, i),
			Cells: []string{
				fmt.Sprintf("%8.3f", float64(e.AtMs)/1000),
				e.Kind, other, e.Detail,
			},
		})
	}
	p.tb.SetRows(rows)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, who)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.tb.Layout(t, gtx, nil)
		}),
	)
}
