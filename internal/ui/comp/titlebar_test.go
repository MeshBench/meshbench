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

// A drag of the bar's body has to report the distance travelled - that is
// how a layer window moves, margins being the only mechanism the protocol
// offers - while the correction a compositor sends once the window has moved
// must not read as a drag back.
func TestTitleBarDragReportsMovement(t *testing.T) {
	b := &TitleBar{Title: "MeshBench - Console"}
	h := newBarHarness(b)
	h.frame()
	h.frame()

	h.press(f32.Pt(20, 16))
	// The pointer moves to (30, 18): a drag of (10, 2), which the caller
	// applies to the window's margins - moving the window under the grabbed
	// pointer, so its surface-relative place is (20, 16) again.
	h.moveTo(f32.Pt(30, 18))
	if d := b.Drag(); d.X != 10 || d.Y != 2 {
		t.Fatalf("first move reported (%d, %d), want (10, 2)", d.X, d.Y)
	}
	// The compositor's next event, at the new surface-relative place, must
	// read as no movement - otherwise every drag would halve itself.
	h.moveTo(f32.Pt(20, 16))
	if d := b.Drag(); d != (image.Point{}) {
		t.Fatalf("the correction event read as a drag of %v; every drag would halve itself", d)
	}
	// And a genuinely further move still reports its full distance.
	h.moveTo(f32.Pt(40, 20))
	if d := b.Drag(); d.X != 20 || d.Y != 4 {
		t.Fatalf("second move reported (%d, %d), want (20, 4)", d.X, d.Y)
	}
	if d := b.Drag(); d != (image.Point{}) {
		t.Fatalf("a second ask reported %v; the drag is consumed", d)
	}
	h.release(f32.Pt(40, 20))
}

// A press on the bar without movement is not a drag: the bar's job is to
// move the window, not to flinch.
func TestTitleBarClickIsNotADrag(t *testing.T) {
	b := &TitleBar{Title: "MeshBench - Console"}
	h := newBarHarness(b)
	h.frame()
	h.frame()
	h.press(f32.Pt(200, 16))
	h.release(f32.Pt(200, 16))
	if d := b.Drag(); d != (image.Point{}) {
		t.Fatalf("a click without movement reported a drag of %v", d)
	}
}
