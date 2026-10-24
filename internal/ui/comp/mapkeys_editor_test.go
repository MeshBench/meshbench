package comp

import (
	"image"

	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/unit"
)

// A text field typed into while the map is on screen keeps its own keys.
//
// Gio hands a key to the first handler in the frame whose filter matches, and a
// filter with no focus target matches whatever is focused. The map is drawn
// before anything that sits over it - a question, a dialog - so its Delete
// shortcut used to take Backspace from the field being typed in: nothing could
// be erased, and with a node selected the key asked to delete the node instead.
func TestBackspaceInAFocusedFieldStaysInTheField(t *testing.T) {
	h := newMapHarness()
	h.sn.Nodes[0].Selected = true
	deleted := false
	h.mv.OnDelete = func([]string) { deleted = true }

	var f Field
	ed := &f.Editor
	ed.SingleLine = true
	ed.SetText("Fifex")
	ed.SetCaret(len("Fifex"), len("Fifex"))
	frame := func(focus bool) {
		h.ops.Reset()
		gtx := layout.Context{
			Ops:         &h.ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(h.sz),
			Source:      h.r.Source(),
		}
		// The map first, as the workbench draws it, then the field over it.
		h.mv.Layout(h.th, gtx, h.sn)
		if focus {
			gtx.Execute(key.FocusCmd{Tag: ed})
		}
		f.Layout(h.th, gtx)
		h.r.Frame(&h.ops)
	}
	frame(true)
	frame(false)

	h.r.Queue(key.Event{Name: key.NameDeleteBackward, State: key.Press})
	frame(false)
	h.r.Queue(key.Event{Name: key.NameDeleteForward, State: key.Press})
	frame(false)

	if deleted {
		t.Error("Backspace in a text field asked the map to delete the selected node")
	}
	if got := ed.Text(); got != "Fife" {
		t.Errorf("Backspace in a focused field left %q, want %q", got, "Fife")
	}
}

// Clicking the map after typing somewhere gives Delete back to the map.
func TestAPressOnTheMapGivesItTheKeysBack(t *testing.T) {
	h := newMapHarness()
	h.sn.Nodes[0].Selected = true
	deleted := false
	h.mv.OnDelete = func([]string) { deleted = true }

	var f Field
	frame := func(focus bool) {
		h.ops.Reset()
		gtx := layout.Context{
			Ops:         &h.ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(h.sz),
			Source:      h.r.Source(),
		}
		h.mv.Layout(h.th, gtx, h.sn)
		if focus {
			gtx.Execute(key.FocusCmd{Tag: &f.Editor})
		}
		// A field-sized field, not one the size of the window: a press on
		// the map has to land on the map.
		gtx.Constraints = layout.Exact(image.Pt(200, 30))
		f.Layout(h.th, gtx)
		h.r.Frame(&h.ops)
	}
	frame(true)
	frame(false)

	// Somewhere empty, well away from the field drawn at the top left.
	h.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: f32.Pt(450, 600), Buttons: pointer.ButtonPrimary},
		pointer.Event{Kind: pointer.Release, Position: f32.Pt(450, 600), Buttons: pointer.ButtonPrimary},
	)
	// A key is routed by the filters of the frame before it, and the press
	// frame read its keys while the field still held focus. The workbench
	// draws every frame, so this is the very next one.
	frame(false)
	frame(false)
	h.r.Queue(key.Event{Name: key.NameDeleteForward, State: key.Press})
	frame(false)
	if !deleted {
		t.Fatal("after a press on the map, Delete still did not reach it")
	}
}
