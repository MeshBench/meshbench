// The last two panels of P6: what the real network is doing (6.20), and how
// far the model is from it (6.18).
//
// Both read the same observed receptions from the deployment. A live feed and
// a validation are the same data asked two questions: what is happening, and
// whether we would have predicted it.
package main

import (
	"fmt"

	"gioui.org/layout"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// feedPanel is recent traffic on the real network (6.20).
type feedPanel struct {
	tb   comp.Table
	init bool
	pull comp.Button
	// OnPull asks the store to fetch recent receptions.
	OnPull func()
}

func (p *feedPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "when", Width: 160, Mono: true, Sortable: true},
			{Title: "heard by", Width: 200, Sortable: true},
			{Title: "from", Width: 200, Sortable: true},
			{Title: "hops", Width: 66, Right: true, Mono: true},
			{Title: "SNR", Width: 80, Right: true, Mono: true, Sortable: true},
			{Title: "packet", Mono: true},
		}
		p.tb.SortCol, p.tb.SortDesc = 0, true
		p.pull.Label, p.pull.Kind = "pull the last hour", comp.Primary
		p.init = true
	}
	if p.pull.Click.Clicked(gtx) && p.OnPull != nil {
		p.OnPull()
	}
	body := func(gtx layout.Context) layout.Dimensions {
		if s == nil || len(s.Observed) == 0 {
			return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
				"nothing pulled yet - this is the real network, not the simulated one"))
		}
		rows := make([]comp.Row, 0, len(s.Observed))
		for i, o := range s.Observed {
			snr := "-"
			if o.HasSNR {
				snr = fmt.Sprintf("%.1f", o.SNRdB)
			}
			rows = append(rows, comp.Row{
				Key: fmt.Sprintf("%d/%s", i, o.PacketID),
				Cells: []string{
					o.At.Format("2006-01-02 15:04:05"),
					o.Receiver, o.Origin, fmt.Sprintf("%d", o.HopCount),
					snr, o.PacketID,
				},
			})
		}
		p.tb.SetRows(rows)
		return p.tb.Layout(t, gtx, nil)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, "the real network")),
		layout.Flexed(1, body),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = 0
			return p.pull.Layout(t, gtx)
		}),
	)
}

// validatePanel is the model against reality (6.18).
//
// Residuals, not a verdict. "The model is 3 dB optimistic on this network" is
// something somebody can act on; "validation failed" is not.
type validatePanel struct {
	tb   comp.Table
	init bool
}

func (p *validatePanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "", Width: 260},
			{Title: "value", Width: 140, Right: true, Mono: true},
			{Title: "what it says"},
		}
		p.init = true
	}
	if s == nil || s.Residuals == nil {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
			"pull the live feed first - residuals need something real to be residuals against"))
	}
	r := s.Residuals
	rows := []comp.Row{
		{Key: "pairs", Cells: []string{"observed pairs matched", fmt.Sprintf("%d", r.Matched),
			"receptions between two nodes this scenario also has"}},
		{Key: "unmatched", Cells: []string{"observed pairs not in the scenario",
			fmt.Sprintf("%d", r.Unmatched),
			"heard on the real network between nodes this scenario does not contain"}},
		{Key: "median", Cells: []string{"median residual", fmt.Sprintf("%+.1f dB", r.MedianDB),
			"positive means the model predicts more margin than was observed"}},
		{Key: "spread", Cells: []string{"half the residuals within",
			fmt.Sprintf("%.1f dB", r.IQRdB),
			"the interquartile range; a wide one means the model is inconsistent, not merely wrong"}},
	}
	p.tb.SetRows(rows)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, "the model against reality")),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
			"residuals, not a verdict: a bias somebody can correct for beats a pass or a fail")),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.tb.Layout(t, gtx, nil)
		}),
	)
}
