package comp

import (
	"fmt"
	"image"
	"math"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// Matrix draws arms against seeds (6.15).
//
// The chart exists to answer one question: did the difference between two arms
// survive reseeding, or is it the seed. A column that is uniformly darker than
// its neighbour is a result; a column that is mottled the same as everything
// else is a run of the random number generator, and no amount of averaging
// afterwards makes it otherwise.
type Matrix struct{}

func (Matrix) Layout(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	sz := gtx.Constraints.Max
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, t.P.Sunk, clip.Rect{Max: sz}.Op())

	m := (*state.Matrix)(nil)
	if s != nil {
		m = s.Matrix
	}
	if m == nil || len(m.Arms) == 0 || len(m.Seeds) == 0 {
		return layout.Center.Layout(gtx, Text(t, t.Sz.Body, t.P.Dim,
			"no sweep loaded - run one, or open a sweep's results"))
	}

	gutter := gtx.Dp(150)
	top := gtx.Dp(34)
	cw := (sz.X - gutter - gtx.Dp(12)) / len(m.Seeds)
	ch := (sz.Y - top - gtx.Dp(30)) / len(m.Arms)
	if cw < 4 || ch < 4 {
		return layout.Center.Layout(gtx, Text(t, t.Sz.Body, t.P.Dim,
			"the panel is too small for this many arms and seeds"))
	}

	// The scale spans the values present, so the colour says how this sweep
	// varied rather than how it compares to some absolute nobody chose.
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, v := range m.Values {
		if math.IsNaN(v) {
			continue
		}
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	if !(hi > lo) {
		hi = lo + 1
	}

	off := op.Offset(image.Pt(gtx.Dp(t.Sp.S), gtx.Dp(t.Sp.S))).Push(gtx.Ops)
	SectionTitle(t, m.Metric+", by arm and seed")(unbounded(gtx))
	off.Pop()

	for r, arm := range m.Arms {
		y := top + r*ch
		lo2 := op.Offset(image.Pt(gtx.Dp(t.Sp.S), y+ch/2-gtx.Dp(7))).Push(gtx.Ops)
		g := gtx
		g.Constraints.Min = image.Point{}
		g.Constraints.Max = image.Pt(gutter-gtx.Dp(t.Sp.M), ch)
		OneLine(t, t.Sz.Caption, t.P.Dim, arm, false)(g)
		lo2.Pop()

		for c := range m.Seeds {
			i := r*len(m.Seeds) + c
			if i >= len(m.Values) {
				continue
			}
			x := gutter + c*cw
			v := m.Values[i]
			col := theme.Alpha(t.P.Faint, 0.25)
			if !math.IsNaN(v) {
				// Cold to warm across the range present.
				u := (v - lo) / (hi - lo)
				col = hsv(210-160*u, 0.55, 0.55+0.35*u)
			}
			paint.FillShape(gtx.Ops, col, clip.Rect{
				Min: image.Pt(x+1, y+1), Max: image.Pt(x+cw-1, y+ch-1)}.Op())
		}
	}

	// Seeds along the bottom, which is where the eye goes to ask "which run
	// was that".
	for c, seed := range m.Seeds {
		x := gutter + c*cw
		off := op.Offset(image.Pt(x+2, top+len(m.Arms)*ch+gtx.Dp(4))).Push(gtx.Ops)
		g := gtx
		g.Constraints.Min = image.Point{}
		g.Constraints.Max = image.Pt(cw-2, gtx.Dp(20))
		OneLine(t, t.Sz.Caption, t.P.Faint, fmt.Sprintf("%d", seed), true)(g)
		off.Pop()
	}
	return layout.Dimensions{Size: sz}
}
