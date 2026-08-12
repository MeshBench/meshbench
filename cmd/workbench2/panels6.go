// Three of P6's panels, each built from what the snapshot already carries.
package main

import (
	"fmt"
	"image/color"

	"gioui.org/layout"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// schedulePanel is the fixture's sends and the claims it makes (6.10).
//
// Both in one panel because they are one thought: what the run does, and what
// it has to be true for the run to have passed. Separating them is how a
// scenario ends up with sends nothing asserts on.
type schedulePanel struct {
	sends  comp.Table
	claims comp.Table
	init   bool
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
		sendRows = append(sendRows, comp.Row{
			Key: fmt.Sprintf("%d/%s", i, snd.Node),
			Cells: []string{
				fmt.Sprintf("%.1fs", float64(snd.AtMs)/1000),
				every, snd.Node, snd.Command,
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
	p.sends.SetRows(sendRows)
	p.claims.SetRows(claimRows)

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

// linkPanel is one link, both directions, always (6.3).
type linkPanel struct{}

func (linkPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if s == nil || len(s.Budgets) == 0 {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
			"select a node to see its strongest link"))
	}
	var kids []layout.FlexChild
	for i := range s.Budgets {
		b := &s.Budgets[i]
		kids = append(kids,
			layout.Rigid(comp.SectionTitle(t, b.From+" to "+b.To)),
			layout.Rigid(comp.Mono(t, t.Sz.Body, verdictColour(t, b.MarginDB),
				fmt.Sprintf("margin %+.1f dB   %s", b.MarginDB, verdictWord(b.MarginDB)))),
		)
		for _, term := range b.Terms {
			kids = append(kids, layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Dim,
				fmt.Sprintf("  %-22s %+8.1f", term.Name, term.DB))))
		}
		kids = append(kids, layout.Rigid(comp.Spacer))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
}

func verdictWord(m float64) string {
	switch {
	case m < 0:
		return "does not close"
	case m < 6:
		return "marginal"
	}
	return "comfortable"
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
