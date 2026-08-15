package comp

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

// BarSegment is one coloured share of a ProportionBar.
type BarSegment struct {
	Frac  float64
	Color color.NRGBA
}

// ProportionBar draws one horizontal bar split into coloured shares, left to
// right in the order given - for showing a total split into named classes at
// a glance, the way a row of percentages in a table cannot.
func ProportionBar(height unit.Dp, segs []BarSegment) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		w := gtx.Constraints.Max.X
		h := gtx.Dp(height)
		x := 0
		for _, s := range segs {
			if s.Frac <= 0 || x >= w {
				continue
			}
			sw := int(float64(w)*s.Frac + 0.5)
			if x+sw > w {
				sw = w - x
			}
			if sw <= 0 {
				continue
			}
			off := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
			FillRect(gtx, image.Pt(sw, h), s.Color)
			off.Pop()
			x += sw
		}
		return layout.Dimensions{Size: image.Pt(w, h)}
	}
}
