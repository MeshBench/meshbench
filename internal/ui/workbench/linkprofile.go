// The Link panel: wb1's cut-through, drawn.
//
// The ground with the earth's curvature in it, the sight line, the first
// Fresnel zone as a band, and the knife edges labelled with what each costs -
// the picture that turns "the link is bad" into "that ridge at 4.2 km costs
// you 31 dB".
package workbench

import (
	"fmt"
	"image"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// The widths the panel changes shape at, and the least room the chart is
// worth drawing in.
//
// A rail is 340dp and a window is a thousand: the same single-row header
// cannot serve both. Below wideHeader the header stacks, because two
// direction margins beside a pair of node names in 340dp left the margins a
// column one character wide, wrapping vertically, and a header tall enough to
// leave the chart no height at all.
const (
	wideHeader = unit.Dp(560)
	minChart   = unit.Dp(150)
)

// linkPanel is the cut-through, its two margins and its verdict.
//
// The list is the panel's own, not one per frame: a scroll position lives at
// the widget's address, and it is what keeps a narrow panel honest - when the
// words and the chart together want more room than the rail has, the panel
// scrolls rather than dropping the chart.
type linkPanel struct {
	list widget.List
}

func (lp *linkPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if s == nil || s.LinkProfile == nil || len(s.LinkProfile.Samples) < 2 {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
			"pick two ends with the map's link tool - nodes, bare ground, or "+
				"one of each - and the ground between them is cut through here"))
	}
	p := s.LinkProfile
	lp.list.Axis = layout.Vertical
	chartH := lp.chartHeight(t, gtx, p)
	tail := lp.notes(t, p, exaggeration(image.Pt(gtx.Constraints.Max.X, chartH), p))
	parts := []layout.Widget{
		lp.header(t, p),
		func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = chartH, chartH
			return drawCutThrough(t, gtx, p)
		},
		tail,
	}
	return comp.List(t, &lp.list, len(parts),
		func(gtx layout.Context, i int) layout.Dimensions {
			return parts[i](gtx)
		})(gtx)
}

// chartHeight is what the picture gets: the panel less the words, and never
// less than the floor.
//
// Measured rather than guessed. The verdict is a sentence and wraps to five
// lines in a rail and one in a window, so a fixed allowance for the words is
// wrong at one width or the other - and the words are measured against the
// panel's whole height first, because the exaggeration they quote is a fact
// about a picture that has not been given its height yet. Where the floor
// wins, the list around all three scrolls: a chart squeezed to nothing is the
// panel saying nothing at all.
func (lp *linkPanel) chartHeight(t *theme.Theme, gtx layout.Context, p *state.Profile) int {
	h := gtx.Constraints.Max.Y - heightOf(gtx, lp.header(t, p)) -
		heightOf(gtx, lp.notes(t, p, exaggeration(gtx.Constraints.Max, p)))
	if min := gtx.Dp(minChart); h < min {
		return min
	}
	return h
}

// heightOf is how tall a widget comes out at this width, drawn into a macro
// that is then thrown away.
func heightOf(gtx layout.Context, w layout.Widget) int {
	macro := op.Record(gtx.Ops)
	gtx.Constraints.Min.Y = 0
	d := w(gtx)
	macro.Stop()
	return d.Size.Y
}

// header is the pair, the distance and both directions' margins.
//
// One row where there is room for one, stacked where there is not. Both
// directions either way: a margin that does not say which way it was measured
// is wrong even when the arithmetic is right.
func (lp *linkPanel) header(t *theme.Theme, p *state.Profile) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		pair := comp.OneLine(t, t.Sz.Section, t.P.Dim, p.From+"  <->  "+p.To, false)
		km := comp.Mono(t, t.Sz.Caption, t.P.Dim, fmt.Sprintf("%.1f km", p.DistanceKm))
		if gtx.Constraints.Max.X < gtx.Dp(wideHeader) {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(pair),
				layout.Rigid(km),
				layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
				layout.Rigid(marginRow(t, p.From, p.To, p.AtoB)),
				layout.Rigid(marginRow(t, p.To, p.From, p.BtoA)),
			)
		}
		// Under a third of the row each, so the pair of names keeps the rest.
		// A header where the two margins take the width is the header this
		// panel started with, and the names were what it left nothing for.
		block := gtx.Constraints.Max.X * 3 / 10
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, pair),
			layout.Rigid(layout.Spacer{Width: t.Sp.M}.Layout),
			layout.Rigid(km),
			layout.Rigid(layout.Spacer{Width: t.Sp.M}.Layout),
			layout.Rigid(marginBlockAt(t, p.From, p.To, p.AtoB, block)),
			layout.Rigid(layout.Spacer{Width: t.Sp.M}.Layout),
			layout.Rigid(marginBlockAt(t, p.To, p.From, p.BtoA, block)),
		)
	}
}

// notes is everything under the chart: what the picture is, what each edge
// costs, the verdict, and what the margins assumed.
func (lp *linkPanel) notes(t *theme.Theme, p *state.Profile, exag float64) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, fmt.Sprintf(
				"terrain includes earth curvature; the band is the first Fresnel "+
					"zone   |   %.1f km, %.0f-%.0f m   |   vertical exaggeration x%.1f",
				p.DistanceKm, p.LowM, p.HighM, exag))),
			layout.Rigid(edgeCosts(t, p)),
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, p.Verdict)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if p.Assumed == "" {
					return layout.Dimensions{}
				}
				// Where the margins came from is content, not small print: a
				// number whose model is silent reads as measured.
				return comp.Text(t, t.Sz.Caption, t.P.Faint,
					"margins assume "+p.Assumed+" - the air will be worse")(gtx)
			}),
		)
	}
}

// edgeCosts is what each knife edge takes, wrapped rather than cut off: in a
// rail this is four lines, and truncating it hides the obstruction that
// decided the link.
func edgeCosts(t *theme.Theme, p *state.Profile) layout.Widget {
	if len(p.Edges) == 0 {
		return comp.Text(t, t.Sz.Caption, t.P.Faint,
			"no obstruction costs more than a decibel")
	}
	parts := make([]string, 0, len(p.Edges))
	for _, e := range p.Edges {
		parts = append(parts, fmt.Sprintf("%.1f km -%.1f dB", e.DistM/1000, e.LossDB))
	}
	return comp.Text(t, t.Sz.Caption, t.P.Warn, "edges:  "+strings.Join(parts, "   "))
}

// marginRow is one direction as a line: who to who, then the decibels, with
// the names giving way first because the number is the answer.
func marginRow(t *theme.Theme, from, to string, db float64) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, t.P.Faint,
				from+" -> "+to, false)),
			layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
			layout.Rigid(comp.Mono(t, t.Sz.Caption, verdictColour(t, db),
				fmt.Sprintf("%+.1f dB", db))),
		)
	}
}

// marginBlockAt is one direction as a stacked block for the wide header,
// bounded so a pair of long names cannot squeeze the other direction out of
// the row entirely.
func marginBlockAt(t *theme.Theme, from, to string, db float64, maxW int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if gtx.Constraints.Max.X > maxW {
			gtx.Constraints.Max.X = maxW
		}
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
			layout.Rigid(comp.OneLine(t, t.Sz.Caption, t.P.Faint, from+" -> "+to, false)),
			layout.Rigid(comp.Mono(t, t.Sz.Caption, verdictColour(t, db),
				fmt.Sprintf("%+.1f dB", db))),
		)
	}
}
