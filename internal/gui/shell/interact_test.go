package shell

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

	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// Interaction, tested rather than eyeballed.
//
// A screenshot proves a control was drawn in the right place. It cannot prove
// that clicking it does anything, and this week the difference mattered: the
// menu bar rendered correctly while the File menu was open on every launch and
// the body it covered was not drawn at all.
//
// These drive the shell through Gio's own input router: lay out a frame, put a
// click where the control was drawn, lay out again, and assert on what
// changed.

// harness is a shell and enough scaffolding to click on it.
type harness struct {
	sh  *Shell
	th  *theme.Theme
	r   input.Router
	ops op.Ops
	sz  image.Point
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	sh := New()
	sh.Add(&Panel{Name: "Map", Windowable: true,
		Draw: func(*theme.Theme, layout.Context, *state.Snapshot) layout.Dimensions {
			return layout.Dimensions{}
		}})
	return &harness{
		sh: sh,
		th: theme.New(theme.Dark, theme.Default,
			text.NewShaper(text.WithCollection(gofont.Collection()))),
		sz: image.Pt(1400, 900),
	}
}

// frame lays the shell out once and hands the ops to the router, which is what
// a window does between frames.
func (h *harness) frame() {
	h.ops.Reset()
	gtx := layout.Context{
		Ops:         &h.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(h.sz),
		Source:      h.r.Source(),
	}
	h.sh.Layout(h.th, gtx, &state.Snapshot{})
	h.r.Frame(&h.ops)
}

// click presses and releases at a point, then lays out again so the widget
// sees it.
func (h *harness) click(at f32.Point) {
	h.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: at, Buttons: pointer.ButtonPrimary},
		pointer.Event{Kind: pointer.Release, Position: at, Buttons: pointer.ButtonPrimary},
	)
	h.frame()
}

// No menu is open when the application starts.
//
// openMenu's zero value is zero, which is a valid menu index. That is not a
// hypothetical: it shipped, and it opened the File menu over the view on every
// launch until somebody looked at a whole window instead of a crop.
func TestNoMenuIsOpenAtStartup(t *testing.T) {
	h := newHarness(t)
	h.frame()
	if h.sh.openMenu != -1 {
		t.Fatalf("menu %d is open before anything was clicked", h.sh.openMenu)
	}
}

// Clicking a menu title opens that menu, and clicking it again closes it.
func TestClickingAMenuOpensAndClosesIt(t *testing.T) {
	h := newHarness(t)
	h.frame()
	// The second menu, so a test that only ever exercises index zero cannot
	// pass by accident.
	at := f32.Pt(float32(h.sh.menus[1].x)+6, 8)

	h.click(at)
	if h.sh.openMenu != 1 {
		t.Fatalf("clicking the second menu opened %d", h.sh.openMenu)
	}
	h.click(at)
	if h.sh.openMenu != -1 {
		t.Fatalf("clicking it again left %d open", h.sh.openMenu)
	}
}

// A menu opens under its own title, not under the first one.
//
// The first attempt positioned dropdowns at 8 + 74*index, which is right for
// one menu and wrong for the rest, because titles are different widths.
func TestEachMenuKnowsWhereItWasDrawn(t *testing.T) {
	h := newHarness(t)
	h.frame()
	prev := -1
	for i := range h.sh.menus {
		x := h.sh.menus[i].x
		if x <= prev {
			t.Fatalf("menu %d (%s) is at x=%d, not to the right of the one before at %d",
				i, h.sh.menus[i].name, x, prev)
		}
		prev = x
	}
}

// Clicking a view tab switches the view. The tabs are how every panel is
// reached, so a tab that does not respond is an interface with one view.
func TestClickingAViewTabSwitchesTheView(t *testing.T) {
	h := newHarness(t)
	h.frame()
	before := h.sh.View
	// The view bar is the second row; a little way in from the left is the
	// first tab, and past it the second.
	h.click(f32.Pt(90, 34))
	if h.sh.View == before {
		t.Fatalf("the view did not change from %v", before)
	}
}
