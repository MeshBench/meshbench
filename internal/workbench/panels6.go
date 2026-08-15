// Three of P6's panels, each built from what the snapshot already carries.
package workbench

import (
	"fmt"
	"image/color"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// schedulePanel is the fixture's sends and the claims it makes (6.10).
//
// Both in one panel because they are one thought: what the run does, and what
// it has to be true for the run to have passed. Separating them is how a
// scenario ends up with sends nothing asserts on.
type schedulePanel struct {
	sends        comp.Table
	claims       comp.Table
	faults       comp.Table
	connectivity comp.Table
	init         bool
}

func (p *schedulePanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.sends.Cols = []comp.Column{
			{Title: "at", Width: 76, Right: true, Mono: true, Sortable: true},
			{Title: "every", Width: 76, Right: true, Mono: true},
			{Title: "node", Width: 170, Sortable: true},
			{Title: "command", Mono: true},
		}
		p.claims.Cols = []comp.Column{
			{Title: "claim", Width: 150, Sortable: true},
			{Title: "node", Width: 170, Sortable: true},
			{Title: "within", Width: 84, Right: true, Mono: true},
			{Title: "bounds", Mono: true},
		}
		p.faults.Cols = []comp.Column{
			{Title: "at", Width: 76, Right: true, Mono: true, Sortable: true},
			{Title: "event", Width: 90, Sortable: true},
			{Title: "node", Width: 170, Sortable: true},
			{Title: "reach before", Width: 110, Right: true, Mono: true},
			{Title: "reach after", Width: 110, Right: true, Mono: true},
			{Title: "recovery", Mono: true},
		}
		p.connectivity.Cols = []comp.Column{
			{Title: "node", Width: 170, Sortable: true},
			{Title: "samples", Width: 84, Right: true, Mono: true},
			{Title: "min neighbours", Width: 110, Right: true, Mono: true, Sortable: true},
			{Title: "longest gap", Mono: true},
		}
		p.init = true
	}
	if s == nil {
		return layout.Dimensions{}
	}

	sendRows := make([]comp.Row, 0, len(s.Sends))
	for i, snd := range s.Sends {
		every := "once"
		if snd.EveryMs > 0 {
			every = fmt.Sprintf("%.1fs", float64(snd.EveryMs)/1000)
		}
		command := snd.Command
		switch {
		case snd.Fault == "move":
			command = fmt.Sprintf("[move to %.4f,%.4f over %.1fs]", snd.ToLat, snd.ToLon,
				float64(snd.DurationMs)/1000)
		case snd.Fault != "":
			// A mutation, not a command - shown in the same table rather
			// than a second one, because "what happens to this scenario and
			// when" is one timeline, not two.
			command = "[fault: " + snd.Fault + "]"
		}
		sendRows = append(sendRows, comp.Row{
			Key: fmt.Sprintf("%d/%s", i, snd.Node),
			Cells: []string{
				fmt.Sprintf("%.1fs", float64(snd.AtMs)/1000),
				every, snd.Node, command,
			},
		})
	}
	claimRows := make([]comp.Row, 0, len(s.Assertions))
	for i, a := range s.Assertions {
		within := ""
		if a.WithinMs > 0 {
			within = fmt.Sprintf("%.0fs", float64(a.WithinMs)/1000)
		}
		claimRows = append(claimRows, comp.Row{
			Key:   fmt.Sprintf("%d/%s", i, a.Kind),
			Cells: []string{a.Kind, a.Node, within, boundsOf(a)},
		})
	}
	faultRows := make([]comp.Row, 0, len(s.FaultLog))
	for i, ev := range s.FaultLog {
		recovery := "watching"
		if ev.Kind != "node-down" && ev.Kind != "depart" {
			recovery = ""
		} else if ev.Recovered {
			recovery = fmt.Sprintf("recovered by %.1fs (%.1fs after) - %d undelivered",
				float64(ev.RecoveredAtMs)/1000,
				float64(ev.RecoveredAtMs-ev.AtMs)/1000, ev.UndeliveredCost)
		} else {
			// Not a blank: a permanent partition has to say so, not look
			// like a recovery nobody has checked on yet.
			recovery = "not recovered"
		}
		faultRows = append(faultRows, comp.Row{
			Key: fmt.Sprintf("%d/%s", i, ev.Node),
			Cells: []string{
				fmt.Sprintf("%.1fs", float64(ev.AtMs)/1000),
				ev.Kind, ev.Node,
				fmt.Sprintf("%d/%d out, %d/%d in", ev.OutBefore, ev.Total, ev.InBefore, ev.Total),
				fmt.Sprintf("%d/%d out, %d/%d in", ev.OutAfter, ev.Total, ev.InAfter, ev.Total),
				recovery,
			},
		})
	}

	connRows := make([]comp.Row, 0, len(s.Connectivity))
	for _, c := range s.Connectivity {
		gap := "none yet"
		if c.LongestGapMs > 0 {
			gap = fmt.Sprintf("%.1fs at %.1fs", float64(c.LongestGapMs)/1000, float64(c.LongestGapAtMs)/1000)
		}
		connRows = append(connRows, comp.Row{
			Key: c.Node,
			Cells: []string{
				c.Node, fmt.Sprintf("%d", c.Samples), fmt.Sprintf("%d", c.MinNeighbours), gap,
			},
		})
	}

	p.sends.SetRows(sendRows)
	p.claims.SetRows(claimRows)
	p.faults.SetRows(faultRows)
	p.connectivity.SetRows(connRows)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, fmt.Sprintf("%d sends", len(sendRows)))),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.sends.Layout(t, gtx, nil)
		}),
		layout.Rigid(comp.SectionTitle(t,
			fmt.Sprintf("%d assertions", len(claimRows)))),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.claims.Layout(t, gtx, nil)
		}),
		layout.Rigid(comp.SectionTitle(t,
			fmt.Sprintf("%d faults fired", len(faultRows)))),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.faults.Layout(t, gtx, nil)
		}),
		layout.Rigid(comp.SectionTitle(t,
			fmt.Sprintf("%d nodes tracked", len(connRows)))),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.connectivity.Layout(t, gtx, nil)
		}),
	)
}

// boundsOf prints only the bounds an assertion actually sets. Printing the
// zero values as though they were limits would turn "at least ten" into "at
// least ten, at most zero", which is not a claim anybody made.
func boundsOf(a state.Assertion) string {
	out := ""
	if a.AtLeast != 0 {
		out += fmt.Sprintf("at least %d  ", a.AtLeast)
	}
	if a.AtMost != 0 {
		out += fmt.Sprintf("at most %d  ", a.AtMost)
	}
	if a.MaxPct != 0 {
		out += fmt.Sprintf("max %.1f%%", a.MaxPct)
	}
	return out
}

func verdictColour(t *theme.Theme, m float64) color.NRGBA {
	switch {
	case m < 0:
		return t.P.Bad
	case m < 6:
		return t.P.Warn
	}
	return t.P.Good
}

// consolePanel is what one node has been doing (6.9).
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
