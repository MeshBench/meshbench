package shell

import (
	"image"

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
	"github.com/MeshBench/meshbench/internal/ui/uitest"
)

type chromeHarness struct {
	bar    comp.TitleBar
	chrome *LayerChrome
	r      input.Router
	ops    op.Ops
	// pxPerDp is the frame's metric, settable because a window dragged onto a
	// screen at another scale has it change underneath the drag.
	pxPerDp float32
	applied []app.Option
	closed  bool
}

func newChromeHarness() *chromeHarness {
	return &chromeHarness{pxPerDp: 2, chrome: NewLayerChrome(float.Spot{
		Top: unit.Dp(24), Left: unit.Dp(24)})}
}

func (h *chromeHarness) frame(sz image.Point) {
	h.ops.Reset()
	gtx := layout.Context{
		Ops:         &h.ops,
		Metric:      unit.Metric{PxPerDp: h.pxPerDp, PxPerSp: h.pxPerDp},
		Constraints: layout.Exact(sz),
		Source:      h.r.Source(),
	}
	// The chrome needs the frame's facts the way the window loop supplies
	// them, so a drag is stated in dp rather than pixels.
	h.chrome.Frame(app.FrameEvent{Size: sz, Metric: gtx.Metric})
	h.bar.Layout(theme.New(theme.Dark, theme.Default,
		text.NewShaper(text.WithCollection(uitest.BrandFaces()))), gtx)
	h.r.Frame(&h.ops)
	opts, close := h.chrome.Update(&h.bar)
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
