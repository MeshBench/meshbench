// A row of steps, with the one being worked on marked.
//
// For a panel whose work is a sequence rather than a set of buttons: fetch
// what was heard, compare it with what was predicted, then correct for the
// difference. Each step says whether it has happened, so the panel answers
// "where am I in this" without anybody having to remember the order.
//
// The state is read from the world, never counted here: a step that marks
// itself done because its button was pressed is a step that lies the moment
// the verb behind it refuses.
package comp

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// Step is one stage of a sequence.
type Step struct {
	// Label is what the step is, in the operator's words.
	Label string
	// Done says it has happened. Now marks the one to do next; at most one
	// step should say so, and the caller decides which by looking at the
	// world rather than at what was last pressed.
	Done bool
	Now  bool
}

// Steps draws the sequence left to right.
func Steps(t *theme.Theme, steps []Step) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		kids := make([]layout.FlexChild, 0, len(steps)*2)
		for i := range steps {
			st := steps[i]
			if i > 0 {
				kids = append(kids, layout.Rigid(
					Text(t, t.Sz.Caption, t.P.Faint, "   >   ")))
			}
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				ink := t.P.Dim
				switch {
				case st.Now:
					ink = t.P.Accent
				case st.Done:
					ink = t.P.Ink
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return stepMark(gtx, t, st)
					}),
					layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout),
					layout.Rigid(Text(t, t.Sz.Caption, ink, st.Label)),
				)
			}))
		}
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
	}
}

// stepMark is the dot beside a step: filled once it has happened, ringed
// while it is the one to do, and hollow before that.
func stepMark(gtx layout.Context, t *theme.Theme, st Step) layout.Dimensions {
	d := gtx.Dp(9)
	box := image.Pt(d, d)
	col := t.P.Faint
	switch {
	case st.Done:
		col = t.P.Good
	case st.Now:
		col = t.P.Accent
	}
	defer clip.Ellipse{Max: box}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: col}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	if !st.Done {
		// Hollow until it has happened, so a glance counts what is left
		// rather than reading four labels.
		in := gtx.Dp(3)
		off := clip.Ellipse{
			Min: image.Pt(in, in), Max: image.Pt(d-in, d-in),
		}.Push(gtx.Ops)
		paint.ColorOp{Color: t.P.Panel}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		off.Pop()
	}
	return layout.Dimensions{Size: box}
}
