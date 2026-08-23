package workbench

import (
	"image"
	"testing"

	"gioui.org/app"
	"gioui.org/f32"
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
// window to drive here, so the tests assert on the options and the chrome's
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
		text.NewShaper(text.WithCollection(brandFaces()))), gtx)
	h.r.Frame(&h.ops)
	opts, close := h.chrome.update(&h.bar)
	h.applied = append(h.applied, opts...)
	// Sticky: a later frame with nothing pressed must not un-close a window
	// that was asked to close.
	h.closed = h.closed || close
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

// The drag that used to be fatal: the window's own move lags the pointer,
// so a position is reported against whichever place the compositor has
// landed by then. A model that accumulates per-event deltas counts each
// pending move again on every event, and the window accelerates off the
// screen; the grab anchor telescopes to the right place instead. The
// harness plays compositor: each frame's requested move lands before the
// next event is generated, so positions are reported against the window's
// latest place - the behaviour the old model could not survive.
func TestLayerChromeDragSurvivesMoveLag(t *testing.T) {
	h := newChromeHarness()
	h.chrome.screens(image.Rect(0, 0, 800, 600), []image.Rectangle{image.Rect(0, 0, 800, 600)})
	h.frame(image.Pt(800, 600))
	h.frame(image.Pt(800, 600))
	h.r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(100, 12), Buttons: pointer.ButtonPrimary})
	h.frame(image.Pt(800, 600))
	// The pointer moves 10px further right every frame for twenty frames.
	landed := float32(0) // how far the window has moved, in pixels
	for i := 1; i <= 20; i++ {
		abs := 100 + 10*float32(i) // the pointer's absolute position
		h.r.Queue(pointer.Event{Kind: pointer.Move,
			Position: f32.Pt(abs-landed, 12), Buttons: pointer.ButtonPrimary})
		before := h.chrome.spot.Left
		h.frame(image.Pt(800, 600))
		// This frame's request lands before the next event.
		landed += (float32(h.chrome.spot.Left) - float32(before)) * 2
	}
	// The pointer travelled 200px = 100dp; the window asked to be exactly
	// under it, however many events that took. The old model reported
	// 100dp per event here, 100 times over.
	if want := unit.Dp(24 + 100); h.chrome.spot.Left != want {
		t.Fatalf("place is %v after 20 landing-lagged events, want %v", h.chrome.spot.Left, want)
	}
}

// A drag cannot carry the window off the top-left corner, nor past the
// output's edges once the fork has said how big the output is: a negative
// margin is a window whose bar nobody can reach, and nothing but a recall
// can drag it back.
func TestLayerChromeDragClamps(t *testing.T) {
	h := newChromeHarness()
	h.chrome.screens(image.Rect(0, 0, 800, 600), []image.Rectangle{image.Rect(0, 0, 800, 600)})
	h.frame(image.Pt(800, 600))
	h.frame(image.Pt(800, 600))
	h.drag(f32.Pt(100, 12), f32.Pt(20, -60))
	if h.chrome.spot.Top != 0 {
		t.Fatalf("top margin is %v after dragging past the corner, want 0", h.chrome.spot.Top)
	}
	if h.chrome.spot.Left < 0 {
		t.Fatalf("left margin is %v, want clamped at 0 or beyond", h.chrome.spot.Left)
	}
	// Right and bottom: dragging far past the output's far edge leaves the
	// bar at its rightmost reachable place, not beyond it.
	h.drag(f32.Pt(400, 12), f32.Pt(10000, 12))
	if max := unit.Dp(800/2 - barGrabDp); h.chrome.spot.Left > max {
		t.Fatalf("left margin is %v, want at most %v (the bar still grabbable)",
			h.chrome.spot.Left, max)
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

// Recall is the escape hatch: whatever the place has become, it puts the
// window back somewhere its bar can be reached from.
func TestLayerChromeRecall(t *testing.T) {
	h := newChromeHarness()
	h.frame(image.Pt(800, 600))
	h.frame(image.Pt(800, 600))
	h.chrome.spot = float.Spot{Top: unit.Dp(5000), Left: unit.Dp(5000)}
	opts := h.chrome.recall(float.Spot{Top: unit.Dp(24), Left: unit.Dp(24)})
	if len(opts) != 1 || h.chrome.spot.Left != unit.Dp(24) {
		t.Fatalf("recall moved nothing: %d options, place %v", len(opts), h.chrome.spot.Left)
	}
	// A maximised window is already full-output and on top; recall declines.
	h.chrome.maximised = true
	if opts := h.chrome.recall(float.Spot{}); opts != nil {
		t.Fatalf("recall of a maximised window applied %d options, want none", len(opts))
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

// The bug this clamp was rewritten for: a pop-out would not move onto the
// screen to its left, and jerked when it arrived on the one to its right.
//
// Margins are measured from the surface's own screen, so reaching the screen
// to the left needs a negative one. Clamping margins at zero forbade that
// along with genuinely leaving the desktop, and clamping them at this screen's
// width undid a rightward move the moment the compositor handed the surface
// over. A direction is only closed off when no screen lies that way.
func TestLayerChromeDragReachesTheScreenNextDoor(t *testing.T) {
	left := image.Rect(0, 0, 2560, 1440)
	right := image.Rect(2560, 0, 4480, 1080)

	// Anchored to the right-hand screen, dragging left towards the other.
	h := newChromeHarness()
	h.chrome.screens(right, []image.Rectangle{left, right})
	h.frame(image.Pt(800, 600))
	h.frame(image.Pt(800, 600))
	h.drag(f32.Pt(100, 12), f32.Pt(-400, 0))
	if h.chrome.spot.Left >= 0 {
		t.Errorf("margin is %v after dragging towards the screen on the left, "+
			"want it negative - that is what reaching the next screen is",
			h.chrome.spot.Left)
	}

	// Anchored to the left-hand screen, dragging right past its own edge.
	h = newChromeHarness()
	h.chrome.screens(left, []image.Rectangle{left, right})
	h.frame(image.Pt(800, 600))
	h.frame(image.Pt(800, 600))
	h.drag(f32.Pt(100, 12), f32.Pt(4000, 0))
	if edge := unit.Dp(2560/2) - 120; h.chrome.spot.Left <= edge {
		t.Errorf("margin is %v after dragging towards the screen on the right, "+
			"want it past this screen's own edge at %v", h.chrome.spot.Left, edge)
	}

	// And with only one screen, the direction is closed off again.
	h = newChromeHarness()
	h.chrome.screens(left, []image.Rectangle{left})
	h.frame(image.Pt(800, 600))
	h.frame(image.Pt(800, 600))
	h.drag(f32.Pt(100, 12), f32.Pt(-400, 0))
	if h.chrome.spot.Left != 0 {
		t.Errorf("margin is %v on a single screen, want 0: there is nothing "+
			"to the left of it", h.chrome.spot.Left)
	}
}
