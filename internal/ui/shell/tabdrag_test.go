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

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// Tabs without dragging are half a dock: they say a region can hold several
// panels and give no way to decide which region a panel is in.

type dragHarness struct {
	sh  *Shell
	th  *theme.Theme
	r   input.Router
	ops op.Ops
	sz  image.Point
}

func newDragHarness() *dragHarness {
	h := &dragHarness{
		sh: New(),
		th: theme.New(theme.Dark, theme.Default,
			text.NewShaper(text.WithCollection(gofont.Collection()))),
		sz: image.Pt(1400, 900),
	}
	for _, n := range []string{"Map", "Nodes", "Inspector", "Waterfall"} {
		h.sh.Add(EmptyPanel(n, "for the drag test"))
	}
	h.sh.PoppedOut = func(string) bool { return false }
	return h
}

func (h *dragHarness) frame() {
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

// drag presses at one point, travels through the middle, and lets go at
// another - the gesture as a hand performs it, one event at a time.
// The router makes the drag events itself, from moves while a button is
// down, so this queues what a mouse actually reports.
func (h *dragHarness) drag(from, to f32.Point) {
	h.r.Queue(pointer.Event{Kind: pointer.Move, Position: from, Source: pointer.Mouse})
	h.frame()
	h.r.Queue(pointer.Event{Kind: pointer.Press, Position: from,
		Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	h.frame()
	for i := 1; i <= 4; i++ {
		f := float32(i) / 4
		at := f32.Pt(from.X+(to.X-from.X)*f, from.Y+(to.Y-from.Y)*f)
		h.r.Queue(pointer.Event{Kind: pointer.Move, Position: at,
			Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
		h.frame()
	}
	h.r.Queue(pointer.Event{Kind: pointer.Release, Position: to,
		Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	h.frame()
}

// where reports which region holds a panel, for asserting a move happened.
func (h *dragHarness) where(name string) regionRef {
	r, _ := h.sh.find(name)
	return r
}

// A tab dragged onto another region moves there, and leaves nothing behind.
func TestDraggingATabMovesThePanel(t *testing.T) {
	h := newDragHarness()
	h.frame()
	h.frame() // the second frame has last frame's geometry to drop against

	from := h.where("Map")
	to := h.where("Inspector")
	if !from.valid() || !to.valid() || from == to {
		t.Fatalf("the Plan view should start with Map and Inspector in different regions, got %v and %v", from, to)
	}
	// The Map tab sits at the top left of its region; the Inspector's region
	// is somewhere in the rail.
	tab := f32.Pt(float32(h.sh.regionRect[from].Min.X+20), float32(h.sh.regionRect[from].Min.Y+10))
	target := h.sh.regionRect[to]
	drop := f32.Pt(float32(target.Min.X+target.Dx()/2), float32(target.Min.Y+target.Dy()/2))

	h.drag(tab, drop)

	if got := h.where("Map"); got != to {
		t.Fatalf("after dragging Map onto the Inspector's region it is in %v, want %v", got, to)
	}
	// And the region it came from no longer lists it.
	if c := h.sh.regionAt(h.sh.View, from); c != nil && indexOf(c.Tabs, "Map") >= 0 {
		t.Fatal("the panel was left behind in the region it was dragged out of")
	}
	if h.sh.drag != nil {
		t.Fatal("the drag is still in progress after the pointer was released")
	}
}

// A press that does not travel is a click, and shows the tab rather than
// moving it. A slightly unsteady hand must not rearrange the window.
func TestAClickOnATabDoesNotMoveIt(t *testing.T) {
	h := newDragHarness()
	h.frame()
	h.frame()
	h.sh.Dock("Waterfall") // two tabs in one region, so there is something to choose
	h.frame()
	h.frame()

	ref := h.where("Waterfall")
	c := h.sh.regionAt(h.sh.View, ref)
	if c == nil || len(c.Tabs) < 2 {
		t.Fatalf("expected a region with two tabs, got %v", c)
	}
	c.Active = len(c.Tabs) - 1
	h.frame()

	// The first tab, pressed and released with a pixel of wobble.
	at := f32.Pt(float32(h.sh.regionRect[ref].Min.X+20), float32(h.sh.regionRect[ref].Min.Y+10))
	h.drag(at, f32.Pt(at.X+1, at.Y+1))

	if got := h.where("Waterfall"); got != ref {
		t.Fatalf("a click moved the panel to %v; only a drag may move one", got)
	}
	if c.Active != 0 {
		t.Fatalf("the pressed tab is not the one showing: active is %d", c.Active)
	}
}

// Dropping a tab back where it started changes nothing.
func TestDroppingATabOnItsOwnRegionIsNoMove(t *testing.T) {
	h := newDragHarness()
	h.frame()
	h.frame()
	ref := h.where("Map")
	r := h.sh.regionRect[ref]
	at := f32.Pt(float32(r.Min.X+20), float32(r.Min.Y+10))
	h.drag(at, f32.Pt(float32(r.Min.X+r.Dx()/2), float32(r.Min.Y+r.Dy()/2)))
	if got := h.where("Map"); got != ref {
		t.Fatalf("dropping a tab on its own region moved it to %v", got)
	}
}

// The cross inside a tab still closes it. The tab owns a pointer area over
// its whole size so a press anywhere can start a drag, and that area sits
// under the cross rather than over it.
func TestTheCrossOnATabStillCloses(t *testing.T) {
	h := newDragHarness()
	h.frame()
	h.frame()
	ref := h.where("Map")
	r := h.sh.regionRect[ref]
	// Along the strip, left to right, until the panel goes. Scanned rather
	// than computed: where the cross sits is the layout's business, and a
	// coordinate written down here is a test of arithmetic.
	y := float32(r.Min.Y + 10)
	for x := float32(r.Min.X + 4); x < float32(r.Min.X+260); x += 3 {
		if !h.sh.Visible("Map") {
			break
		}
		h.r.Queue(
			pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, y),
				Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
			pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, y),
				Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
		)
		h.frame()
	}
	if h.sh.Visible("Map") {
		t.Fatal("no press along the tab strip reached the cross")
	}
}

// A right-click on a tab sends that panel to a window of its own, whichever
// tab is showing, and picks nothing up on the way.
func TestRightClickingATabSendsItToItsOwnWindow(t *testing.T) {
	h := newDragHarness()
	var popped []string
	h.sh.OnPopOut = func(name string) { popped = append(popped, name) }
	h.frame()
	h.frame()
	h.sh.Dock("Waterfall") // a second tab, so the one right-clicked is not the shown one
	h.frame()
	h.frame()

	ref := h.where("Map")
	c := h.sh.regionAt(h.sh.View, ref)
	if c == nil || len(c.Tabs) < 2 || c.Tabs[0] != "Map" {
		t.Fatalf("expected Map first in a region of two tabs, got %v", c)
	}
	c.Active = 1
	h.frame()

	r := h.sh.regionRect[ref]
	at := f32.Pt(float32(r.Min.X+20), float32(r.Min.Y+10))
	h.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: at,
			Buttons: pointer.ButtonSecondary, Source: pointer.Mouse},
		pointer.Event{Kind: pointer.Release, Position: at,
			Buttons: pointer.ButtonSecondary, Source: pointer.Mouse},
	)
	h.frame()

	if len(popped) != 1 || popped[0] != "Map" {
		t.Fatalf("a right-click on the Map tab popped out %v, want just Map", popped)
	}
	if h.sh.drag != nil {
		t.Fatal("a right-click picked the tab up; a secondary press is not a gesture")
	}
	if c.Active != 1 {
		t.Fatalf("a right-click changed which tab is showing, to %d", c.Active)
	}
}
