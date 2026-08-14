// The tables of P4, on the virtualised component from P1.
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
	"gioui.org/unit"

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

func eventTint(t *theme.Theme, kind string) [4]uint8 {
	var c = t.P.Dim
	switch kind {
	case "tx":
		c = t.P.Accent
	case "rx":
		c = t.P.Good
	case "miss":
		c = t.P.Bad
	}
	return [4]uint8{c.R, c.G, c.B, 255}
}

// scorePanel is the per-node counters (6.8).
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

// withCount puts a line under a table saying how much of the truth it shows.
//
// Only when it is showing less than all of it. A table that always announces
// its own completeness trains somebody to stop reading the line, which is
// exactly when it matters.
func withCount(t *theme.Theme, gtx layout.Context, d layout.Dimensions,
	label string, capped bool) layout.Dimensions {

	if !capped {
		return d
	}
	col := t.P.Warn
	off := layout.Inset{Top: unit.Dp(2)}
	_ = off
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return d }),
		layout.Rigid(comp.Text(t, t.Sz.Caption, col, label+
			" - the rest are older than the tail the interface keeps")),
	)
}
