// The selected row, said in full.
//
// The last field is the one worth the most and cost the least: what the board's
// own profile says this line does. Those sentences were written by whoever
// transcribed the board from the vendor's pinout, they are some of the best
// prose in the tree, and until now they were visible only to somebody reading
// Go. A firmware developer staring at a radio that hears nothing should not
// have to open the source to find the paragraph explaining why.
package bringup

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

func (p *Panel) inspector(t *theme.Theme, gtx layout.Context, rows []Row) layout.Dimensions {
	return onPanel(t, gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(t.Sp.M).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if len(rows) == 0 || p.sel >= len(rows) {
				return comp.Text(t, t.Sz.Caption, t.P.Faint, "nothing selected")(gtx)
			}
			r := rows[p.sel]
			kids := []layout.FlexChild{
				layout.Rigid(field(t, "Selected")),
				layout.Rigid(comp.Text(t, t.Sz.Body, t.P.Ink, r.Name)),
			}
			if r.Where != "" {
				kids = append(kids,
					layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Dim, r.Where)))
			}
			kids = append(kids,
				layout.Rigid(field(t, "Declared")),
				layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Ink, orDash(r.Declared))),
				layout.Rigid(field(t, "Observed")),
				layout.Rigid(comp.Text(t, t.Sz.Caption, r.Verdict.Colour(t),
					orDash(r.Observed))),
			)
			// Said rather than left to the colour, where there is a verdict to
			// say: a reader who has to learn the palette to decode one is a
			// reader who gets half of them wrong.
			if v := r.Verdict.String(); v != "" {
				kids = append(kids, layout.Rigid(
					comp.Mono(t, t.Sz.Caption, r.Verdict.Colour(t), v)))
			}

			if r.Why != "" {
				kids = append(kids,
					layout.Rigid(field(t, "Why this matters")),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return quoted(t, gtx, r.Why)
					}),
					layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
					layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
						"— this board's own profile")),
				)
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
		})
	})
}

// field is one of the inspector's labels.
func field(t *theme.Theme, s string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: t.Sp.S, Bottom: t.Sp.XXS}.Layout(gtx,
			comp.Text(t, t.Sz.Caption, t.P.Faint, upper(s)))
	}
}

// quoted draws the profile's own words behind a rule, so they read as
// something quoted rather than as the interface's opinion.
//
// The bar is as tall as the text beside it, and how tall that is depends on
// how the text wrapped - so the text is laid out into a macro first, the bar is
// drawn to the height that came back, and the text is replayed over it.
func quoted(t *theme.Theme, gtx layout.Context, s string) layout.Dimensions {
	bar := gtx.Dp(unit.Dp(2))
	gap := gtx.Dp(t.Sp.S)

	inner := gtx
	inner.Constraints.Max.X -= bar + gap
	if inner.Constraints.Max.X < 1 {
		inner.Constraints.Max.X = 1
	}
	inner.Constraints.Min.X = 0

	macro := op.Record(gtx.Ops)
	dims := comp.Text(t, t.Sz.Caption, t.P.Dim, s)(inner)
	call := macro.Stop()

	paint.FillShape(gtx.Ops, t.P.Accent,
		clip.Rect{Max: image.Pt(bar, dims.Size.Y)}.Op())
	defer op.Offset(image.Pt(bar+gap, 0)).Push(gtx.Ops).Pop()
	call.Add(gtx.Ops)
	return layout.Dimensions{Size: image.Pt(bar+gap+dims.Size.X, dims.Size.Y)}
}
