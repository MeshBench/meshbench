package comp

import (
	"fmt"
	"image"
	"math"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// Budget draws a link budget as a cascade (6.4).
//
// A cascade rather than a bar chart of the terms: each bar starts where the
// last one ended, so the shape shows the running total falling from transmit
// power through the path loss and back up, and the gap at the end above zero
// is the margin. A row of independent bars would show the same five numbers
// and none of the story.
type Budget struct{}

func (Budget) Layout(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	sz := gtx.Constraints.Max
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, t.P.Sunk, clip.Rect{Max: sz}.Op())

	if s == nil || len(s.Budgets) == 0 {
		return layout.Center.Layout(gtx, Text(t, t.Sz.Body, t.P.Dim,
			"select a node to see its strongest link's budget, both ways"))
	}
	// Both directions, side by side, because a link is two links and the panel
	// that shows one of them is the panel that hides the asymmetry.
	half := sz.X / len(s.Budgets)
	for i := range s.Budgets {
		off := op.Offset(image.Pt(i*half, 0)).Push(gtx.Ops)
		g := gtx
		g.Constraints = layout.Exact(image.Pt(half, sz.Y))
		drawCascade(t, g, &s.Budgets[i])
		off.Pop()
	}
	return layout.Dimensions{Size: sz}
}

func drawCascade(t *theme.Theme, gtx layout.Context, b *state.Budget) {
	sz := gtx.Constraints.Max
	pad := gtx.Dp(t.Sp.M)
	top := gtx.Dp(30)

	off := op.Offset(image.Pt(pad, gtx.Dp(t.Sp.S))).Push(gtx.Ops)
	SectionTitle(t, b.From+" to "+b.To)(unbounded(gtx))
	off.Pop()

	// The vertical scale spans every value the running total takes, so nothing
	// is drawn off the panel and the zero line is where it belongs.
	lo, hi := 0.0, 0.0
	run := 0.0
	for _, term := range b.Terms {
		run += term.DB
		lo, hi = math.Min(lo, run), math.Max(hi, run)
	}
	if hi-lo < 1 {
		return
	}
	plotH := float64(sz.Y - top - gtx.Dp(46))
	y := func(v float64) int { return top + int((hi-v)/(hi-lo)*plotH) }

	// Zero: the line a margin is measured against.
	zero := y(0)
	paint.FillShape(gtx.Ops, theme.Alpha(t.P.Ink, 0.35), clip.Rect{
		Min: image.Pt(pad, zero), Max: image.Pt(sz.X-pad, zero+1)}.Op())

	w := (sz.X - 2*pad) / max(1, len(b.Terms))
	run = 0
	for i, term := range b.Terms {
		from, to := run, run+term.DB
		run = to
		x0 := pad + i*w
		y0, y1 := y(math.Max(from, to)), y(math.Min(from, to))
		if y1-y0 < 2 {
			y1 = y0 + 2
		}
		col := t.P.Good
		if term.DB < 0 {
			col = t.P.Bad
		}
		paint.FillShape(gtx.Ops, theme.Alpha(col, 0.75), clip.Rect{
			Min: image.Pt(x0+2, y0), Max: image.Pt(x0+w-2, y1)}.Op())

		// The name under the bar, rotated is not worth it at this size: cut
		// instead, and the value is the thing being read anyway.
		lo := op.Offset(image.Pt(x0+2, sz.Y-gtx.Dp(40))).Push(gtx.Ops)
		g := gtx
		g.Constraints.Min = image.Point{}
		g.Constraints.Max = image.Pt(w-4, gtx.Dp(30))
		OneLine(t, t.Sz.Caption, t.P.Faint, term.Name, false)(g)
		lo.Pop()

		vo := op.Offset(image.Pt(x0+2, sz.Y-gtx.Dp(24))).Push(gtx.Ops)
		OneLine(t, t.Sz.Caption, t.P.Dim, fmt.Sprintf("%+.1f", term.DB), true)(g)
		vo.Pop()
	}

	// The answer, said in words as well as drawn.
	verdict := fmt.Sprintf("margin %+.1f dB", b.MarginDB)
	col := t.P.Good
	if b.MarginDB < 0 {
		verdict += " - does not close"
		col = t.P.Bad
	} else if b.MarginDB < 6 {
		verdict += " - marginal"
		col = t.P.Warn
	}
	vo := op.Offset(image.Pt(pad, sz.Y-gtx.Dp(60))).Push(gtx.Ops)
	Mono(t, t.Sz.Body, col, verdict)(unbounded(gtx))
	vo.Pop()
}
