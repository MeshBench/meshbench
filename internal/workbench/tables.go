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
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.M}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return airtimeBreakdown(t, gtx, s.Scores)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.tb.Layout(t, gtx, nil)
		}),
	)
}

// airtimeBreakdown is the network-wide "where is the air going" summary:
// cards for the totals, and a bar so two runs with equal totals and
// different redundancy look different at a glance rather than only on
// careful reading.
//
// A message reaching a node that already had it costs exactly as much air
// as one that does not - the number this exists to make visible.
func airtimeBreakdown(t *theme.Theme, gtx layout.Context, scores []state.Score) layout.Dimensions {
	var total, payload, overhead, redundant float64
	var nowMs float64
	for _, v := range scores {
		total += v.AirtimeMs
		payload += v.AirtimePayloadMs
		overhead += v.AirtimeOverheadMs
		redundant += v.AirtimeRedundantMs
		if v.DutyCyclePct > 0 {
			// Back out the run length from any one node's own duty cycle,
			// rather than threading NowMs through another layer - they all
			// measured the same clock.
			nowMs = 100 * v.AirtimeMs / v.DutyCyclePct
		}
	}
	if total <= 0 {
		return comp.Text(t, t.Sz.Caption, t.P.Faint,
			"nothing has transmitted yet")(gtx)
	}
	pct := func(v float64) float64 { return 100 * v / total }
	busy := ""
	if nowMs > 0 {
		busy = fmt.Sprintf("channel busy %.1f%%", 100*total/nowMs)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return comp.CellGrid(t, gtx, 160, []layout.Widget{
				comp.StatCell(t, "on air", fmt.Sprintf("%.0f s", total/1000), busy),
				comp.StatCell(t, "payload", fmt.Sprintf("%.0f%%  %.0f s", pct(payload), payload/1000),
					"reached somewhere new"),
				comp.StatCell(t, "overhead", fmt.Sprintf("%.0f%%  %.0f s", pct(overhead), overhead/1000),
					"adverts, acks, path bytes"),
				comp.StatCell(t, "redundant", fmt.Sprintf("%.0f%%  %.0f s", pct(redundant), redundant/1000),
					"relayed to receivers who already had it"),
			})
		}),
		layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
		layout.Rigid(comp.ProportionBar(6, []comp.BarSegment{
			{Frac: payload / total, Color: t.P.Accent},
			{Frac: overhead / total, Color: t.P.Faint},
			{Frac: redundant / total, Color: t.P.Warn},
		})),
	)
}
