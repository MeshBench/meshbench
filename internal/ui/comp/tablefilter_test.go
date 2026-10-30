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

// A table whose caller draws the filter does not draw it again: one editor
// laid out twice is two boxes with the same text and two carets.
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
