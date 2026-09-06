// A row of things that becomes two rows when one will not do.
//
// Chips are the case this exists for. A rail narrower than the row of them
// laid the last chip out at twenty pixels with its label folded in half and
// sliced the one after it in two, so four of the eight event-class filters
// could not be pressed at all while the panel was docked - and nothing said
// so, because the counts they filter were all present in the cards above.
//
// Not CellGrid, which divides the width by a minimum and gives every cell the
// same track: chips are as wide as their words, and "All events" beside "Sent"
// in equal columns wastes most of a narrow rail.
package comp

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

// Flow lays widgets left to right, wrapping to a new line when the next one
// would not fit, with gap between them both ways.
//
// Each is measured at the full width available rather than at what is left of
// the line, so a widget decides its own size and this decides where it goes.
// One wider than the whole line still gets a line of its own rather than being
// squeezed into nothing.
func Flow(gtx layout.Context, gap unit.Dp, items ...layout.Widget) layout.Dimensions {
	if len(items) == 0 {
		return layout.Dimensions{}
	}
	g := gtx.Dp(gap)
	maxW := gtx.Constraints.Max.X

	// Measured first, placed second: a macro per item, so nothing is laid out
	// twice and the widths are known before the first line is committed.
	type placed struct {
		call op.CallOp
		size image.Point
	}
	drawn := make([]placed, 0, len(items))
	for _, w := range items {
		mgtx := gtx
		mgtx.Constraints.Min = image.Point{}
		macro := op.Record(gtx.Ops)
		d := w(mgtx)
		drawn = append(drawn, placed{call: macro.Stop(), size: d.Size})
	}

	x, y, lineH, widest := 0, 0, 0, 0
	for _, p := range drawn {
		if x > 0 && x+p.size.X > maxW {
			x, y = 0, y+lineH+g
			lineH = 0
		}
		off := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
		p.call.Add(gtx.Ops)
		off.Pop()
		x += p.size.X + g
		if x-g > widest {
			widest = x - g
		}
		if p.size.Y > lineH {
			lineH = p.size.Y
		}
	}
	return layout.Dimensions{Size: image.Pt(widest, y+lineH)}
}
