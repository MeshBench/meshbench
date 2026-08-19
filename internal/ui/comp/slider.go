// A slider, in the theme's own colours.
//
// material.Slider was the one control in the interface drawn from Gio's
// material palette rather than theme.Theme. This is the same widget.Float
// underneath, with a themed track and thumb over it.
package comp

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// Slider selects a value in [0, 1] by dragging.
type Slider struct {
	Float widget.Float
	// Default fills the value once, before anybody has touched the control,
	// so a slider can start at 0.75 while zero stays reachable ever after.
	// Re-filling whenever the value read zero made "drag it to nothing"
	// impossible: the control snapped back every frame.
	Default float32
	inited  bool
}

// Value is the slider's position, with the default applied - safe to read
// before the control has ever been drawn.
func (s *Slider) Value() float32 {
	if !s.inited {
		return s.Default
	}
	return s.Float.Value
}

// Layout draws the slider horizontally, filling the minimum width it is given.
func (s *Slider) Layout(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	if !s.inited {
		s.Float.Value, s.inited = s.Default, true
	}
	thumb := gtx.Dp(6)
	track := gtx.Dp(2)
	width := gtx.Constraints.Min.X
	if width < 6*thumb {
		width = 6 * thumb
	}
	height := 2*thumb + gtx.Dp(2)

	// The draggable strip, inset by the thumb's radius so its centre can
	// reach both ends of the track.
	off := op.Offset(image.Pt(thumb, 0)).Push(gtx.Ops)
	gtx.Constraints.Min = image.Pt(width-2*thumb, height)
	dims := s.Float.Layout(gtx, layout.Horizontal, 6)
	off.Pop()

	cy := height / 2
	at := thumb + int(s.Float.Value*float32(dims.Size.X))

	tOff := op.Offset(image.Pt(thumb, cy-track/2)).Push(gtx.Ops)
	FillRect(gtx, image.Pt(width-2*thumb, track), t.P.Rule)
	tOff.Pop()
	if at > thumb {
		fOff := op.Offset(image.Pt(thumb, cy-track/2)).Push(gtx.Ops)
		FillRect(gtx, image.Pt(at-thumb, track), t.P.Accent)
		fOff.Pop()
	}
	// The thumb: a circle, so its radius is half its size rather than a
	// constant that could outgrow it.
	nOff := op.Offset(image.Pt(at-thumb, cy-thumb)).Push(gtx.Ops)
	RoundRect(gtx, image.Pt(2*thumb, 2*thumb), 6, t.P.Accent)
	nOff.Pop()

	return layout.Dimensions{Size: image.Pt(width, height)}
}
