// The Schedule panel: the fixture's sends, and the claims it makes.
//
// Both in one panel because they are one thought - what the run does, and what
// has to be true for the run to have passed. Separating them is how a scenario
// ends up with sends nothing asserts on.
package workbench

import (
	"fmt"
	"image/color"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// schedulePanel is the fixture's sends and the claims it makes.
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

func verdictColour(t *theme.Theme, m float64) color.NRGBA {
	switch {
	case m < 0:
		return t.P.Bad
	case m < 6:
		return t.P.Warn
	}
	return t.P.Good
}
