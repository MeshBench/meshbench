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

type sliderHarness struct {
	s   *Slider
	th  *theme.Theme
	r   input.Router
	ops op.Ops
	sz  image.Point
}

func newSliderHarness() *sliderHarness {
	return &sliderHarness{
		s: &Slider{Default: 0.75},
		th: theme.New(theme.Dark, theme.Default,
			text.NewShaper(text.WithCollection(gofont.Collection()))),
		sz: image.Pt(200, 20),
	}
}

func (h *sliderHarness) frame() {
	h.ops.Reset()
	gtx := layout.Context{
		Ops:         &h.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(h.sz),
		Source:      h.r.Source(),
	}
	h.s.Layout(h.th, gtx)
	h.r.Frame(&h.ops)
}

func (h *sliderHarness) dragTo(x float32) {
	h.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, 10), Buttons: pointer.ButtonPrimary},
		pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, 10), Buttons: pointer.ButtonPrimary},
	)
	h.frame()
	h.frame()
}

// Zero has to be a position the operator can reach. The old opacity slider
// refilled its default whenever the value read zero, so dragging fully left
// snapped back to 0.75 every frame.
func TestSliderCanBeDraggedToZeroAndStay(t *testing.T) {
	h := newSliderHarness()
	h.frame()
	if got := h.s.Value(); got != 0.75 {
		t.Fatalf("before anybody touched it the slider reads %v, want its default 0.75", got)
	}
	h.dragTo(2)
	if got := h.s.Value(); got != 0 {
		t.Fatalf("dragged fully left the slider reads %v; zero must be reachable", got)
	}
	h.frame()
	h.frame()
	if got := h.s.Value(); got != 0 {
		t.Fatalf("zero did not survive redrawing: the slider snapped back to %v", got)
	}
	h.dragTo(198)
	if got := h.s.Value(); got != 1 {
		t.Fatalf("dragged fully right the slider reads %v, want 1", got)
	}
}

// Value is safe before the first draw: it answers with the default rather
// than zero, so a reader that runs before the control has been laid out does
// not see a value nobody chose.
func TestSliderValueBeforeFirstDraw(t *testing.T) {
	s := &Slider{Default: 0.75}
	if got := s.Value(); got != 0.75 {
		t.Fatalf("unlaid-out slider reads %v, want its default", got)
	}
}
