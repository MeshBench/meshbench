package comp

import (
	"fmt"
	"image"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// Energy draws a year of state of charge at one site (6.19).
//
// The daily minimum rather than the daily mean, because a pack that averages
// half full and empties every night at three does not work, and the mean is
// exactly the number that hides it.
type Energy struct{}

func (Energy) Layout(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	sz := gtx.Constraints.Max
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, t.P.Sunk, clip.Rect{Max: sz}.Op())

	var e *state.Energy
	if s != nil {
		e = s.Energy
	}
	if e == nil || len(e.SoC) == 0 {
		return layout.Center.Layout(gtx, Text(t, t.Sz.Body, t.P.Dim,
			"select a node and run the site study to see its year"))
	}

	pad := gtx.Dp(t.Sp.M)
	top := gtx.Dp(46)
	plotH := sz.Y - top - gtx.Dp(44)
	plotW := sz.X - 2*pad
	if plotH < 30 || plotW < 60 {
		return layout.Dimensions{Size: sz}
	}
	y := func(soc float64) float32 { return float32(top) + float32((1-soc)*float64(plotH)) }
	x := func(day int) float32 {
		return float32(pad) + float32(day)/float32(len(e.SoC)-1)*float32(plotW)
	}

	// The zero line, and the line below which a pack is in trouble. Drawn
	// because "how low did it get" is the only question this chart answers.
	for _, level := range []struct {
		at  float64
		col colorNRGBA
	}{{0.2, t.P.Bad}, {0.5, theme.Alpha(t.P.Rule, 0.8)}, {1, theme.Alpha(t.P.Rule, 0.8)}} {
		yy := int(y(level.at))
		paint.FillShape(gtx.Ops, level.col, clip.Rect{
			Min: image.Pt(pad, yy), Max: image.Pt(sz.X-pad, yy+1)}.Op())
	}

	var path clip.Path
	path.Begin(gtx.Ops)
	n := 0
	prev := f32.Pt(x(0), y(e.SoC[0]))
	for i := 1; i < len(e.SoC); i++ {
		cur := f32.Pt(x(i), y(e.SoC[i]))
		segment(&path, prev, cur, 1.6)
		prev = cur
		n++
	}
	spec := path.End()
	if n > 0 {
		col := t.P.Good
		if e.DeadDays > 0 {
			col = t.P.Bad
		} else if e.WorstSoC < 0.3 {
			col = t.P.Warn
		}
		paint.FillShape(gtx.Ops, col, clip.Outline{Path: spec}.Op())
	}

	// The worst day, marked, because it is the only day that decides whether
	// the site works.
	if e.WorstDay >= 0 && e.WorstDay < len(e.SoC) {
		wx := int(x(e.WorstDay))
		paint.FillShape(gtx.Ops, theme.Alpha(t.P.Warn, 0.8), clip.Rect{
			Min: image.Pt(wx, top), Max: image.Pt(wx+1, top+plotH)}.Op())
	}

	off := op.Offset(image.Pt(pad, gtx.Dp(t.Sp.S))).Push(gtx.Ops)
	SectionTitle(t, "a year at "+e.Node)(unbounded(gtx))
	off.Pop()

	verdict := fmt.Sprintf(
		"worst state of charge %.0f%% on day %d    %.1f days of autonomy    duty %.2f%%",
		e.WorstSoC*100, e.WorstDay, e.AutonomyDays, e.DutyPct)
	col := t.P.Good
	if e.DeadDays > 0 {
		verdict = fmt.Sprintf("dead on %d days - this site does not work    %s",
			e.DeadDays, verdict)
		col = t.P.Bad
	}
	off = op.Offset(image.Pt(pad, gtx.Dp(26))).Push(gtx.Ops)
	Mono(t, t.Sz.Caption, col, verdict)(unbounded(gtx))
	off.Pop()

	off = op.Offset(image.Pt(pad, top+plotH+gtx.Dp(6))).Push(gtx.Ops)
	Mono(t, t.Sz.Caption, t.P.Faint,
		"1 January to 31 December, daily minimum state of charge; the line at 20% is where a pack is in trouble")(
		unbounded(gtx))
	off.Pop()
	return layout.Dimensions{Size: sz}
}
