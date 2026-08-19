// The Score panel: the per-node counters, on the virtualised table.
//
// Each one is a projection of the snapshot into rows, done fresh every frame.
// That sounds wasteful and is not: the table only builds the rows it can show,
// so the cost is the projection, and the alternative - caching rows and
// invalidating them - is a second copy of the truth waiting to disagree with
// the first.
package workbench

import (
	"fmt"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// The events panel lives in events2.go, redesigned around causes.

// snrOf prints the signal-to-noise ratio only where one was measured. A
// transmission has no SNR - it is the receiver that has one - and printing
// 0.0 dB for it would be a measurement that never happened.
func snrOf(e *state.Event) string {
	if e.Kind == "tx" {
		return ""
	}
	return fmt.Sprintf("%.1f", e.SNRdB)
}

// scorePanel is the per-node counters.
type scorePanel struct {
	tb   comp.Table
	init bool
}

func (p *scorePanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "node", Width: 190, Sortable: true},
			{Title: "sent", Width: 66, Right: true, Mono: true, Sortable: true},
			{Title: "heard", Width: 70, Right: true, Mono: true, Sortable: true},
			{Title: "airtime s", Width: 88, Right: true, Mono: true, Sortable: true},
			{Title: "duty %", Width: 74, Right: true, Mono: true, Sortable: true},
			{Title: "delivered", Width: 92, Right: true, Mono: true, Sortable: true},
			{Title: "redundant", Right: true, Mono: true, Sortable: true},
		}
		p.tb.SortCol, p.tb.SortDesc, p.init = 1, true, true
	}
	if s == nil {
		return layout.Dimensions{}
	}
	rows := make([]comp.Row, 0, len(s.Scores))
	for _, v := range s.Scores {
		rows = append(rows, comp.Row{
			Key: v.Name,
			Cells: []string{
				v.Name,
				fmt.Sprintf("%d", v.Sent), fmt.Sprintf("%d", v.Heard),
				fmt.Sprintf("%.2f", v.AirtimeMs/1000),
				fmt.Sprintf("%.2f", v.DutyCyclePct),
				fmt.Sprintf("%d", v.UniqueDelivery),
				fmt.Sprintf("%d", v.RedundantRelay),
			},
		})
	}
	p.tb.SetRows(rows)
	return p.tb.Layout(t, gtx, nil)
}
