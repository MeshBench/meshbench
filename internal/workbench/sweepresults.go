// Reading a sweep back: the per-arm table, the run matrix, and the estimate
// of what a plan will cost before anybody commits to it.
package workbench

import (
	"fmt"
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// sweepResults is what the arms came back with, and whether it is a result.
//
// Stateless on purpose: Draw recomputes everything from the *state.Snapshot
// it is handed each frame, so there is nothing here for a receiver to cache
// between calls.
type sweepResults struct{}

func (p *sweepResults) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if s == nil {
		return layout.Dimensions{}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if s.ExperimentWarning == "" {
				return layout.Dimensions{}
			}
			// Said above the numbers, not below them: a warning underneath a
			// table is read after somebody has already believed it.
			return layout.Inset{Bottom: t.Sp.S}.Layout(gtx,
				comp.OneLine(t, t.Sz.Body, t.P.Warn,
					"not a result yet: "+s.ExperimentWarning, false))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if s.ExperimentVerdict == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Bottom: t.Sp.S}.Layout(gtx,
				comp.OneLine(t, t.Sz.Body, t.P.Ink, s.ExperimentVerdict, false))
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(s.Experiment) == 0 {
				return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint,
					"define arms and seeds above, then run it"))
			}
			return p.matrix(t, gtx, s)
		}),
	)
}

// matrix draws the arms against the first of them.
//
// Every cell but the baseline's shows only how far it moved, because that is
// the question: six columns of absolute figures make the reader do the
// subtraction, and they do it on the two arms that happen to be adjacent.
func (p *sweepResults) matrix(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	base := s.Experiment[0]

	cell := func(text string, colour color.NRGBA, w int, mono bool) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			px := gtx.Dp(unit.Dp(w))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = px, px
			d := comp.OneLine(t, t.Sz.Body, colour, text, mono)(gtx)
			// Forced to the declared width: a cell that measures itself slides
			// every column after it out from under its own header.
			d.Size.X = px
			return d
		})
	}

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			head := make([]layout.FlexChild, 0, len(armCols))
			for _, c := range armCols {
				head = append(head, cell(c.title, t.P.Dim, c.width, false))
			}
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, head...)
		}),
		layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
		layout.Rigid(comp.HRule(t)),
		layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
	}

	for i, a := range s.Experiment {
		i, a := i, a
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			row := make([]layout.FlexChild, 0, len(armCols))
			for _, c := range armCols {
				switch {
				case c.value == nil:
					row = append(row, cell(a.Arm, t.P.Ink, c.width, false))
				case i == 0 || c.better == 0:
					row = append(row, cell(
						fmt.Sprintf("%.0f%s", c.value(a), c.unit), t.P.Ink, c.width, true))
				default:
					ref := c.value(base)
					if ref == 0 {
						row = append(row, cell(
							fmt.Sprintf("%.0f%s", c.value(a), c.unit), t.P.Ink, c.width, true))
						continue
					}
					d := (c.value(a) - ref) / ref * 100
					colour := t.P.Dim
					switch {
					case d > 0.5:
						colour = t.P.Bad
					case d < -0.5:
						colour = t.P.Good
					}
					// Flipped where more is the good direction. Without this a
					// firmware that delivered more would be painted red.
					if c.better > 0 && d > 0.5 {
						colour = t.P.Good
					} else if c.better > 0 && d < -0.5 {
						colour = t.P.Bad
					}
					row = append(row, cell(fmt.Sprintf("%+.1f%%", d), colour, c.width, true))
				}
			}
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, row...)
		}))
	}
	children = append(children,
		layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
		layout.Rigid(comp.OneLine(t, t.Sz.Caption, t.P.Faint,
			"against "+base.Arm+"; every column but rx and delivered is a cost, "+
				"so red is worse in both directions", false)))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// armList draws the chosen arms, each with a way to take it off again.
func (c *sweepControls) armList(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	if len(c.armsNow()) == 0 {
		return comp.Text(t, t.Sz.Caption, t.P.Faint,
			"no arms yet - add one, or cross a parameter across them")(gtx)
	}
	// Capped and scrolling. Six arms crossed from two parameters is already
	// taller than the panel, and a list that grows without bound pushes
	// everything under it off the window with no way back.
	arms := c.armsNow()
	return capped(t, gtx, &c.armScroll, len(arms), 200, func(gtx layout.Context, i int) layout.Dimensions {
		a := arms[i]
		if c.armRm[a] == nil {
			c.armRm[a] = &comp.Button{Label: "x", Kind: comp.Quiet}
		}
		b := c.armRm[a]
		return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return b.Layout(t, gtx)
				}),
				layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
				layout.Flexed(1, comp.OneLine(t, t.Sz.Body, t.P.Ink, a, true)),
			)
		})
	})
}

// capped draws a list inside a bounded, scrolling area.
func capped(t *theme.Theme, gtx layout.Context, l *widget.List, n, maxDp int,
	row func(layout.Context, int) layout.Dimensions) layout.Dimensions {
	h := gtx.Dp(unit.Dp(maxDp))
	if gtx.Constraints.Max.Y < h {
		h = gtx.Constraints.Max.Y
	}
	gtx.Constraints.Max.Y = h
	return comp.List(t, l, n, row)(gtx)
}

// armCol is one column of the matrix.
//
// Better says which way is good, because it decides the colour of a delta and
// getting it backwards makes a regression look like a win. Most of these are
// costs - transmissions, collisions, airtime - where less is better, which is
// the opposite of the convention most tables use.
type armCol struct {
	title  string
	width  int
	value  func(state.ArmSummary) float64
	unit   string
	better int // -1 less is better, +1 more is better, 0 no delta
}

var armCols = []armCol{
	{title: "arm", width: 230},
	{title: "runs", width: 60,
		value: func(a state.ArmSummary) float64 { return float64(a.Runs) }},
	{title: "tx", width: 90, better: -1,
		value: func(a state.ArmSummary) float64 { return a.TX }},
	{title: "rx", width: 90, better: +1,
		value: func(a state.ArmSummary) float64 { return a.RX }},
	{title: "delivered", width: 100, better: +1,
		value: func(a state.ArmSummary) float64 { return a.Delivered }},
	{title: "redundant", width: 100, better: -1,
		value: func(a state.ArmSummary) float64 { return a.Redundant }},
	{title: "collisions", width: 100, better: -1,
		value: func(a state.ArmSummary) float64 { return a.Collided }},
	{title: "airtime", width: 100, unit: " s", better: -1,
		value: func(a state.ArmSummary) float64 { return a.AirtimeMs / 1000 }},
	{title: "seed spread", width: 110,
		value: func(a state.ArmSummary) float64 { return a.RXSpread * 100 }, unit: "%"},
}

// estimate says what pressing "run it" is about to cost, before it is pressed.
func (c *sweepControls) estimate(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	seeds := len(splitFields(fieldText(&c.seeds)))
	arms := len(c.armsNow())
	if seeds == 0 || arms == 0 {
		return layout.Dimensions{}
	}
	runs := arms * seeds
	line := fmt.Sprintf("%d arms x %d seeds = %d runs", arms, seeds, runs)
	if secs, ok := num(&c.runFor); ok && secs > 0 {
		line += fmt.Sprintf(", about %s", roughDuration(runs*int(secs)))
	}
	return layout.Inset{Top: t.Sp.S}.Layout(gtx,
		comp.Text(t, t.Sz.Caption, t.P.Dim, line))
}

// roughDuration is minutes and seconds, because a sweep is measured in the
// first and nobody waits on the second.
func roughDuration(secs int) string {
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	return fmt.Sprintf("%dm%02ds", secs/60, secs%60)
}
