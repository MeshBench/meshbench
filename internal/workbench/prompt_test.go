package workbench

import (
	"testing"

	"gioui.org/f32"
	"gioui.org/layout"

	"github.com/A13xB0/meshcoresim/internal/gui/shell"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// The question a menu entry asks, driven by pointing at it.
//
// "Save this network" fired a verb that wanted a name, got nothing, and failed
// into the status bar. The prompt is what closes that, so the thing to prove is
// not that it draws: it is that a name typed into it reaches the verb.
func TestTheSavePromptDeliversWhatWasTyped(t *testing.T) {
	var p shell.Prompt
	got := ""
	p.Open("Save this network as", "a name", "", func(a string) { got = a })

	h := newPanelHarness(func(th *theme.Theme, gtx layout.Context,
		_ *state.Snapshot) layout.Dimensions {
		return p.Layout(th, gtx)
	}, nil)
	h.frame()

	if !p.Showing() {
		t.Fatal("the question closed itself before anybody could answer it")
	}
	// The field sits under the title, in the middle of the window.
	h.click(f32.Pt(float32(h.sz.X)/2, float32(h.sz.Y)/2))
	h.typeText("fife-strict")

	// OK is the rightmost button on the bottom row of the card. Scanned from
	// the right edge inward rather than computed - and inward matters, because
	// Cancel sits beside it and a left-to-right scan closes the question
	// before it reaches the button that answers it.
	for y := float32(h.sz.Y) / 2; y < float32(h.sz.Y) && got == "" && p.Showing(); y += 3 {
		for x := float32(h.sz.X)/2 + 250; x > float32(h.sz.X)/2+180 && got == ""; x -= 4 {
			h.click(f32.Pt(x, y))
		}
	}

	if got != "fife-strict" {
		t.Fatalf("the verb was handed %q; typing a name into the question and "+
			"pressing OK has to reach it", got)
	}
	if p.Showing() {
		t.Error("the question stayed open after it was answered")
	}
}
