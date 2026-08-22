package workbench

import (
	"image"
	"testing"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/font/gofont"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/float"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// The chrome under a layer window's drawn title bar: the machinery that
// turns the bar's drags and glyph presses into window options. It has no
// window to drive here, so the test asserts on the options and the chrome's
// own state - the parts that can be wrong, the window only applying what it
// is given.

type chromeHarness struct {
	bar     comp.TitleBar
	chrome  *layerChrome
	r       input.Router
	ops     op.Ops
	applied []app.Option
	closed  bool
}

func newChromeHarness() *chromeHarness {
	return &chromeHarness{chrome: newLayerChrome(float.Spot{
		Top: unit.Dp(24), Left: unit.Dp(24)})}
}

func (h *chromeHarness) frame(sz image.Point) {
	h.ops.Reset()
	gtx := layout.Context{
		Ops:         &h.ops,
		Metric:      unit.Metric{PxPerDp: 2, PxPerSp: 2},
		Constraints: layout.Exact(sz),
		Source:      h.r.Source(),
	}
	// The chrome needs the frame's facts the way the window loop supplies
	// them, so a drag is stated in dp rather than pixels.
	h.chrome.frame(app.FrameEvent{Size: sz, Metric: gtx.Metric})
	h.bar.Layout(theme.New(theme.Dark, theme.Default,
		text.NewShaper(text.WithCollection(gofont.Collection()))), gtx)
	h.r.Frame(&h.ops)
	opts, close := h.chrome.update(&h.bar)
	h.applied = append(h.applied, opts...)
	// Sticky: a later frame with nothing pressed must not un-close a window
	// that was asked to close.
	h.closed = h.closed || close
}

// glyphAt returns a point inside the named glyph's cell. The cells are
// square, the height of the bar, at its right end: close last, maximise
// before it. Computed from the theme rather than hardcoded, because the
// harness draws at two pixels per dp and the bar height is the theme's word.
func (h *chromeHarness) glyphAt(which string) f32.Point {
	th := theme.New(theme.Dark, theme.Default, nil)
	barH := 2 * int(th.RowHeight())
	switch which {
	case "maximise":
		return f32.Pt(800-float32(barH)-float32(barH)/2, float32(barH)/2)
	default:
		return f32.Pt(800-float32(barH)/2, float32(barH)/2)
	}
}

// drag presses the bar's body at from and moves to to, the way the router
// synthesises a drag: press, move, release.
func (h *chromeHarness) drag(from, to f32.Point) {
	h.r.Queue(pointer.Event{Kind: pointer.Press, Position: from, Buttons: pointer.ButtonPrimary})
	h.frame(image.Pt(800, 600))
	h.r.Queue(pointer.Event{Kind: pointer.Move, Position: to, Buttons: pointer.ButtonPrimary})
	h.frame(image.Pt(800, 600))
	h.r.Queue(pointer.Event{Kind: pointer.Release, Position: to, Buttons: pointer.ButtonPrimary})
	h.frame(image.Pt(800, 600))
}

func (h *chromeHarness) press(at f32.Point) {
	h.r.Queue(pointer.Event{Kind: pointer.Press, Position: at, Buttons: pointer.ButtonPrimary})
	h.frame(image.Pt(800, 600))
	h.r.Queue(pointer.Event{Kind: pointer.Release, Position: at, Buttons: pointer.ButtonPrimary})
	h.frame(image.Pt(800, 600))
}

// A drag of the bar moves the window's place by the distance travelled, in
// dp: margins position a layer window, so this is the whole of dragging.
func TestLayerChromeDragMoves(t *testing.T) {
	h := newChromeHarness()
	h.frame(image.Pt(800, 600))
	h.frame(image.Pt(800, 600))
	h.drag(f32.Pt(100, 12), f32.Pt(140, 20))
	if len(h.applied) != 1 {
		t.Fatalf("drag applied %d options, want 1 (the move)", len(h.applied))
	}
	spot := float.Spot{Top: unit.Dp(24 + 4), Left: unit.Dp(24 + 20)}
	if h.chrome.spot != spot {
		t.Fatalf("place is (%v, %v), want (%v, %v)",
			h.chrome.spot.Left, h.chrome.spot.Top, spot.Left, spot.Top)
	}
}

// A drag cannot carry the window off the top-left corner: a negative margin
// is a window whose bar nobody can reach, and nothing can drag it back.
func TestLayerChromeDragClamps(t *testing.T) {
	h := newChromeHarness()
	h.frame(image.Pt(800, 600))
	h.frame(image.Pt(800, 600))
	h.drag(f32.Pt(100, 12), f32.Pt(20, -60))
	if h.chrome.spot.Top != 0 {
		t.Fatalf("top margin is %v after dragging past the corner, want 0", h.chrome.spot.Top)
	}
	if h.chrome.spot.Left < 0 {
		t.Fatalf("left margin is %v, want clamped at 0 or beyond", h.chrome.spot.Left)
	}
}

// Maximise anchors to all four edges; restore gives back the place and the
// size that were taken, and a maximised window's bar does not drag.
func TestLayerChromeMaximiseAndRestore(t *testing.T) {
	h := newChromeHarness()
	h.frame(image.Pt(800, 600))
	h.frame(image.Pt(800, 600))
	h.press(h.glyphAt("maximise"))
	if !h.chrome.maximised || len(h.applied) != 1 {
		t.Fatalf("maximise glyph: maximised=%v, %d options applied", h.chrome.maximised, len(h.applied))
	}
	// The bar reports no drag while maximised - the maximise glyph is a
	// restore glyph, and a maximised window has no place to drag to.
	h.drag(f32.Pt(100, 12), f32.Pt(160, 30))
	if len(h.applied) != 1 {
		t.Fatalf("a maximised bar moved the window: %d options applied", len(h.applied))
	}
	h.press(h.glyphAt("maximise"))
	if h.chrome.maximised || len(h.applied) != 3 {
		t.Fatalf("restore glyph: maximised=%v, %d options applied (want 3: the move and the size)",
			h.chrome.maximised, len(h.applied))
	}
	// The restore says where, and gives back the size taken, in dp against
	// the frame's metric.
	if h.chrome.spot != (float.Spot{Top: unit.Dp(24), Left: unit.Dp(24)}) {
		t.Fatalf("restore place is (%v, %v), want the one taken",
			h.chrome.spot.Left, h.chrome.spot.Top)
	}
	if h.chrome.restore.w != unit.Dp(400) || h.chrome.restore.h != unit.Dp(300) {
		t.Fatalf("restore size is (%v, %v), want (400, 300) dp",
			h.chrome.restore.w, h.chrome.restore.h)
	}
}

// Close is close: one press of the glyph, one ask.
func TestLayerChromeClose(t *testing.T) {
	h := newChromeHarness()
	h.frame(image.Pt(800, 600))
	h.frame(image.Pt(800, 600))
	h.press(h.glyphAt("close"))
	if !h.closed {
		t.Fatal("the close glyph never fired; a layer-shell window with no " +
			"decoration and no working close affordance cannot be closed at all")
	}
}
