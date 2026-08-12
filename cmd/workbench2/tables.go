// The tables of P4, on the virtualised component from P1.
//
// Each one is a projection of the snapshot into rows, done fresh every frame.
// That sounds wasteful and is not: the table only builds the rows it can show,
// so the cost is the projection, and the alternative - caching rows and
// invalidating them - is a second copy of the truth waiting to disagree with
// the first.
package main

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// eventsPanel is every event with its cause (6.7).
type eventsPanel struct {
	tb   comp.Table
	init bool
}

func (p *eventsPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "at", Width: 74, Right: true, Mono: true, Sortable: true},
			{Title: "kind", Width: 52, Sortable: true},
			{Title: "from", Width: 150, Sortable: true},
			{Title: "to", Width: 150, Sortable: true},
			{Title: "message", Width: 92, Mono: true, Right: true},
			{Title: "SNR", Width: 66, Right: true, Mono: true, Sortable: true},
			{Title: "detail"},
		}
		// Newest first: an event table is read from the end.
		p.tb.SortCol, p.tb.SortDesc, p.init = 0, true, true
	}
	if s == nil {
		return layout.Dimensions{}
	}
	rows := make([]comp.Row, 0, len(s.Events))
	for i := range s.Events {
		e := &s.Events[i]
		rows = append(rows, comp.Row{
			// Packet and index together: two events of one packet are two
			// rows, and a key that collides makes a table flicker.
			Key: fmt.Sprintf("%d/%d", e.PacketID, i),
			Cells: []string{
				fmt.Sprintf("%8.3f", float64(e.AtMs)/1000),
				e.Kind, e.From, e.To,
				fmt.Sprintf("%d", e.MessageID%100000),
				snrOf(e), e.Detail,
			},
			Tint: eventTint(t, e.Kind),
		})
	}
	p.tb.SetRows(rows)
	return withCount(t, gtx, p.tb.Layout(t, gtx, nil),
		fmt.Sprintf("%d of %d events", len(s.Events), s.EventTotal),
		s.EventTotal > len(s.Events))
}

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
