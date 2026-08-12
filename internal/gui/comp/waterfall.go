package comp

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// Waterfall draws a captured spectrogram (6.5).
//
// Time downwards, frequency across, which is the convention every radio
// operator already has in their hands. The image is uploaded once per capture
// rather than per frame: a capture is a fixed 200 ms window, so redrawing it
// costs one textured quad however long somebody looks at it.
type Waterfall struct {
	op    paint.ImageOp
	forOp string
}

func (w *Waterfall) Layout(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	sz := gtx.Constraints.Max
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, t.P.Sunk, clip.Rect{Max: sz}.Op())

	if s == nil || s.Waterfall == nil || s.Waterfall.Image == nil {
		note := "no capture yet - Simulation, capture the waterfall"
		if s != nil && s.WaterfallNote != "" {
			note = s.WaterfallNote
		}
		return layout.Center.Layout(gtx, Text(t, t.Sz.Body, t.P.Dim, note))
	}

	c := s.Waterfall
	if w.forOp != c.Node || w.op.Size().X == 0 {
		w.op = paint.NewImageOp(c.Image)
		w.forOp = c.Node
	}
	b := c.Image.Bounds()
	// Fills the panel: a spectrogram has no natural size on screen, only a
	// natural orientation.
	plot := image.Pt(sz.X, sz.Y-gtx.Dp(22))
	if plot.Y < 20 {
		plot = sz
	}
	cl := clip.Rect{Max: plot}.Push(gtx.Ops)
	sc := op.Affine(f32Scale(
		float32(plot.X)/float32(b.Dx()),
		float32(plot.Y)/float32(b.Dy()))).Push(gtx.Ops)
	w.op.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	sc.Pop()
	cl.Pop()

	off := op.Offset(image.Pt(gtx.Dp(t.Sp.S), plot.Y+gtx.Dp(4))).Push(gtx.Ops)
	Mono(t, t.Sz.Caption, t.P.Faint,
		"at "+c.Node+"    time downwards, frequency across    colour is dB above the noise floor")(
		unbounded(gtx))
	off.Pop()
	return layout.Dimensions{Size: sz}
}
