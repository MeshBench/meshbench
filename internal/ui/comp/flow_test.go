package comp

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

// A block of a fixed size, for measuring where Flow puts things.
func block(w, h int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(w, h)}
	}
}

func flowIn(width int, items ...layout.Widget) layout.Dimensions {
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(width, 1000)),
	}
	gtx.Constraints.Min = image.Point{}
	return Flow(gtx, 0, items...)
}

// Things that do not fit on one line go onto the next, rather than being
// squeezed until their labels fold in half.
func TestFlowWrapsRatherThanSqueezing(t *testing.T) {
	one := flowIn(300, block(100, 20), block(100, 20), block(100, 20))
	if one.Size.Y != 20 {
		t.Errorf("three 100px items in 300px should be one line 20 tall; got %v",
			one.Size)
	}

	two := flowIn(250, block(100, 20), block(100, 20), block(100, 20))
	if two.Size.Y != 40 {
		t.Errorf("three 100px items in 250px should wrap to two lines 40 tall; "+
			"got %v - a row that does not wrap is a row whose last chips "+
			"cannot be pressed", two.Size)
	}
}

// One wider than the whole line gets a line to itself rather than being
// crushed into what is left of the current one.
func TestFlowGivesAnOversizedItemItsOwnLine(t *testing.T) {
	d := flowIn(150, block(100, 20), block(400, 20))
	if d.Size.Y != 40 {
		t.Errorf("an item wider than the line should start a new one; got %v",
			d.Size)
	}
}

// Nothing in, nothing out - and no panic.
func TestFlowOfNothing(t *testing.T) {
	if d := flowIn(300); d.Size != (image.Point{}) {
		t.Errorf("an empty flow should measure zero; got %v", d.Size)
	}
}
