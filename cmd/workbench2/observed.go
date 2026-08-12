// The last two panels of P6: what the real network is doing (6.20), and how
// far the model is from it (6.18).
//
// Both read the same observed receptions from the deployment. A live feed and
// a validation are the same data asked two questions: what is happening, and
// whether we would have predicted it.
package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"gioui.org/layout"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
	"github.com/A13xB0/meshcoresim/internal/provider"
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

// pullObserved fetches recent receptions from a deployment.
func pullObserved(ctx context.Context, url string, since time.Duration) ([]state.Observed, error) {
	if url == "" {
		return nil, fmt.Errorf("no deployment URL")
	}
	cs := &provider.CoreScope{BaseURL: url}
	recs, err := cs.Receptions(ctx, time.Now().Add(-since))
	if err != nil {
		return nil, err
	}
	out := make([]state.Observed, 0, len(recs))
	for _, r := range recs {
		out = append(out, state.Observed{
			At: r.At, Receiver: r.Receiver, Origin: r.Origin,
			HopCount: r.HopCount, HasSNR: r.HasSNR, SNRdB: r.SNRdB,
			PacketID: r.PacketID,
		})
	}
	return out, nil
}

// residualsOf compares predicted margins against observed signal-to-noise.
//
// Only direct receptions - hop count zero or one - because a packet that
// arrived over three relays says nothing about the link between its origin and
// whoever finally heard it, and counting it would compare a path to a hop.
func (s *sim) residualsOf(obs []state.Observed, links []state.Link,
	nodes []state.Node) *state.Residuals {

	index := map[string]int{}
	for i := range nodes {
		index[nodes[i].Name] = i
	}
	margin := map[[2]int]float64{}
	for _, l := range links {
		if l.Known {
			margin[[2]int{l.A, l.B}] = l.MarginDB
			margin[[2]int{l.B, l.A}] = l.MarginDB
		}
	}
	var diffs []float64
	matched, unmatched := 0, 0
	for _, o := range obs {
		if !o.HasSNR || o.HopCount > 1 {
			continue
		}
		a, ok1 := index[o.Origin]
		b, ok2 := index[o.Receiver]
		if !ok1 || !ok2 {
			unmatched++
			continue
		}
		m, ok := margin[[2]int{a, b}]
		if !ok {
			unmatched++
			continue
		}
		// The observed margin is how far the signal was above what the
		// demodulator needed, which is the same quantity the model predicts.
		observed := o.SNRdB - requiredSNRFor(s, a)
		diffs = append(diffs, m-observed)
		matched++
	}
	if matched == 0 {
		return &state.Residuals{Matched: 0, Unmatched: unmatched}
	}
	sort.Float64s(diffs)
	return &state.Residuals{
		Matched: matched, Unmatched: unmatched,
		MedianDB: diffs[len(diffs)/2],
		IQRdB:    math.Abs(quantile(diffs, 0.75) - quantile(diffs, 0.25)),
	}
}

func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(q * float64(len(sorted)-1))
	return sorted[i]
}

func requiredSNRFor(s *sim, i int) float64 {
	if i < 0 || i >= len(s.nodes) {
		return -15
	}
	switch s.nodes[i].Radio.SpreadFactor {
	case 7:
		return -7.5
	case 8:
		return -10
	case 9:
		return -12.5
	case 11:
		return -17.5
	case 12:
		return -20
	}
	return -15
}
