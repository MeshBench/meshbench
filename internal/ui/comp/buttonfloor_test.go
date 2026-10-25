package comp

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/ui/theme"
	"github.com/MeshBench/meshbench/internal/ui/theme/brandfont"
)

// A squeezed button is a small button, never no button.
//
// Clipping the label rather than folding it left a button with no room for
// even the ellipsis drawing nothing at all: an empty rounded outline, measured
// at zero pixels wide. The events panel's pause button did exactly that in a
// narrow rail. A control nobody can see is one nobody knows to look for, which
// is worse than the folded label it replaced.
func TestAButtonNeverMeasuresZero(t *testing.T) {
	th := theme.New(theme.Dark, theme.Default,
		text.NewShaper(text.WithCollection(brandfont.Collection())))
	for _, w := range []int{0, 4, 12, 30, 60, 200} {
		b := &Button{Label: "pause"}
		gtx := layout.Context{
			Ops:         new(op.Ops),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Max: image.Pt(w, 100)},
		}
		d := b.Layout(th, gtx)
		if d.Size.X == 0 {
			t.Errorf("offered %d px the button drew nothing: an empty outline "+
				"is not a control", w)
		}
	}
}

// A table whose caller draws the filter does not draw it again: one editor
// laid out twice is two boxes with the same text and two carets (#770).
func TestATableDoesNotDrawAFilterItsCallerOwns(t *testing.T) {
	th := theme.New(theme.Dark, theme.Default,
		text.NewShaper(text.WithCollection(brandfont.Collection())))
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(400, 100)},
	}
	tb := &Table{}
	if d := tb.searchRow(th, gtx); d.Size.Y == 0 {
		t.Fatal("a table with its own search drew no search box")
	}
	tb.OwnFilter = true
	if d := tb.searchRow(th, gtx); d.Size.Y != 0 {
		t.Errorf("the table drew the caller's filter again, %d px tall", d.Size.Y)
	}
}
