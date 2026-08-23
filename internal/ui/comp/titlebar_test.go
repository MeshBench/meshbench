package comp

import (
	"image"
	"testing"

	"gioui.org/f32"
	"gioui.org/font/gofont"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// barHarness draws a TitleBar at a fixed size and lets a test press and drag
// it. The pointer events are queued before the frame, the way Gio delivers
// them, and the bar answers on the frame after. Drag events are not queued
// directly - the router synthesises them from a press and the moves that
// follow it - so a drag is press, moves, release.
type barHarness struct {
	bar *TitleBar
	r   input.Router
	ops op.Ops
	sz  image.Point
	th  *theme.Theme
}

func newBarHarness(b *TitleBar) *barHarness {
	return &barHarness{
		bar: b,
		sz:  image.Pt(400, 32),
		th: theme.New(theme.Dark, theme.Default,
			text.NewShaper(text.WithCollection(gofont.Collection()))),
	}
}

func (h *barHarness) frame() {
	h.ops.Reset()
	gtx := layout.Context{
		Ops:         &h.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(h.sz),
		Source:      h.r.Source(),
	}
	h.bar.Layout(h.th, gtx)
	h.r.Frame(&h.ops)
}

func (h *barHarness) press(at f32.Point) {
	h.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: at, Buttons: pointer.ButtonPrimary},
	)
	h.frame()
}

func (h *barHarness) moveTo(at f32.Point) {
	h.r.Queue(pointer.Event{Kind: pointer.Move, Position: at, Buttons: pointer.ButtonPrimary})
	h.frame()
}

func (h *barHarness) release(at f32.Point) {
	h.r.Queue(
		pointer.Event{Kind: pointer.Release, Position: at, Buttons: pointer.ButtonPrimary},
	)
	h.frame()
}

// The glyphs have to both exist and be where a pointer can find them: a bar
// that cannot be closed is a window that cannot be closed.
func TestTitleBarGlyphsAreClickable(t *testing.T) {
	b := &TitleBar{Title: "MeshBench - Console"}
	h := newBarHarness(b)
	h.frame()
	h.frame()

	// The close cell is the top-right corner; maximise beside it.
	h.press(f32.Pt(float32(h.sz.X)-8, 16))
	h.release(f32.Pt(float32(h.sz.X)-8, 16))
	if !b.CloseClicked() {
		t.Fatal("pressing the close glyph never registered")
	}
	if b.CloseClicked() {
		t.Fatal("the close glyph fired twice for one press")
	}
	h.press(f32.Pt(float32(h.sz.X)-32, 16))
	h.release(f32.Pt(float32(h.sz.X)-32, 16))
	if !b.MaximiseClicked() {
		t.Fatal("pressing the maximise glyph never registered")
	}
}

// A drag of the bar's body reports the grab point and the pointer's latest
// position - the two numbers the caller places the window from. The latest
// event wins within a frame's burst, because the caller recomputes the
// window's whole place from them; per-event deltas would double-count every
// event that arrived while the window's own move was still in flight. The
// positions are in the handle's own coordinate space, which the caller
// never minds: it moves by their difference.
func TestTitleBarDragReportsMovement(t *testing.T) {
	b := &TitleBar{Title: "MeshBench - Console"}
	h := newBarHarness(b)
	h.frame()
	h.frame()

	h.press(f32.Pt(20, 16))
	h.moveTo(f32.Pt(30, 18))
	grab, pos, held, fresh := b.Drag()
	if !held || !fresh {
		t.Fatal("the bar is not held, or the event was not reported fresh")
	}
	if d := pos.Sub(grab); d != (f32.Pt(10, 2)) {
		t.Fatalf("drag is %v, want (10, 2)", d)
	}
	// Asking again without an event in between reports nothing fresh: a
	// caller polling every frame must not re-apply the last event.
	if _, _, _, fresh := b.Drag(); fresh {
		t.Fatal("a second ask with no event reported fresh")
	}
	// A burst of further moves: only the latest matters.
	h.moveTo(f32.Pt(40, 20))
	h.moveTo(f32.Pt(50, 22))
	grab2, pos, _, _ := b.Drag()
	if grab2 != grab {
		t.Fatalf("grab moved from %v to %v mid-drag", grab, grab2)
	}
	if d := pos.Sub(grab); d != (f32.Pt(30, 6)) {
		t.Fatalf("drag is %v after a burst, want (30, 6) - the last event only", d)
	}
	// After release the hold is off, though the positions stand so a caller
	// reading every frame sees its target stand still rather than snap back.
	h.release(f32.Pt(50, 22))
	if _, _, held, _ := b.Drag(); held {
		t.Fatal("the bar reports itself held after release")
	}
}

// A press on the bar without movement is not a drag: the grab point and the
// position coincide, so the caller computes no movement.
func TestTitleBarClickIsNotADrag(t *testing.T) {
	b := &TitleBar{Title: "MeshBench - Console"}
	h := newBarHarness(b)
	h.frame()
	h.frame()
	h.press(f32.Pt(200, 16))
	h.release(f32.Pt(200, 16))
	grab, pos, held, _ := b.Drag()
	if held {
		t.Fatal("a click without movement holds the bar")
	}
	if grab != pos {
		t.Fatalf("grab %v and position %v differ after a click without movement", grab, pos)
	}
}
