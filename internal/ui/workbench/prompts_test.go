package workbench

import (
	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/shell"
	"github.com/MeshBench/meshbench/internal/ui/theme"
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

// chooserHarness opens a chooser over the panel harness and hands back both.
func chooserHarness(choices []string, got *string) (*shell.Prompt, *panelHarness) {
	p := &shell.Prompt{}
	p.Choose("Pick one", "filter", choices, func(a string) { *got = a })
	h := newPanelHarness(func(th *theme.Theme, gtx layout.Context,
		_ *state.Snapshot) layout.Dimensions {
		return p.Layout(th, gtx)
	}, nil)
	h.frame() // the first frame focuses the field, so typing needs no click
	return p, h
}

func pressKey(h *panelHarness, name key.Name) {
	h.r.Queue(key.Event{Name: name, State: key.Press})
	h.frame()
}

// Enter must answer with the highlighted choice, never the filter text: typing
// "carto" and pressing Enter used to hand the verb the string "carto", which
// is not one of the things the chooser offered.
func TestChooserEnterPicksTheHighlightedChoice(t *testing.T) {
	got := ""
	p, h := chooserHarness([]string{"carto-dark", "carto-light", "osm"}, &got)
	h.typeText("carto")
	pressKey(h, key.NameReturn)
	if got != "carto-dark" {
		t.Fatalf("Enter answered %q; it must pick the highlighted choice, not the filter text", got)
	}
	if p.Showing() {
		t.Error("the question stayed open after it was answered")
	}
}

// The arrow keys move the highlight, so a keyboard can reach every choice.
func TestChooserArrowsMoveTheAnswer(t *testing.T) {
	got := ""
	_, h := chooserHarness([]string{"carto-dark", "carto-light", "osm"}, &got)
	pressKey(h, key.NameDownArrow)
	pressKey(h, key.NameDownArrow)
	pressKey(h, key.NameUpArrow)
	pressKey(h, key.NameReturn)
	if got != "carto-light" {
		t.Fatalf("down, down, up then Enter answered %q, want the second choice", got)
	}
}

// With nothing matching there is nothing to pick: Enter must not answer, and
// the question must stay open saying "nothing matches that".
func TestChooserEnterWithNoMatchAnswersNothing(t *testing.T) {
	got := ""
	p, h := chooserHarness([]string{"carto-dark", "carto-light", "osm"}, &got)
	h.typeText("zzz")
	pressKey(h, key.NameReturn)
	if got != "" {
		t.Fatalf("Enter with no matching choice answered %q; there was nothing to pick", got)
	}
	if !p.Showing() {
		t.Error("the question closed itself with nothing answered")
	}
}
