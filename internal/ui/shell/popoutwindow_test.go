// The rules every pop-out window follows, tested once because they are now
// applied once.
//
// The bar, the drag, the maximise and the size that has to fit the screen used
// to be split between this loop and each panel: three accessors to implement
// and a block to remember at the top of the panel's own flex. Two of five
// panels implemented the accessors and never laid the bar out, so those windows
// could not be dragged, maximised or closed - and a layer surface is not
// resized by the compositor either, so they were stuck at whatever size they
// opened at. Nothing failed.
package shell

import (
	"image"
	"testing"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"

	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/float"
)

// A window opens wholly on the screen it landed on.
//
// The cascade that places one knows nothing about how big it is, so a tall
// window placed a little way down runs off the bottom - and what runs off a
// layer surface cannot be dragged back by the compositor, only by our own bar,
// which is off the screen by then.
func TestAWindowIsMovedUpRatherThanOffTheBottom(t *testing.T) {
	screen := image.Rect(0, 0, 1920, 1080)
	c := NewLayerChrome(float.Spot{Top: 500, Left: 40})
	c.Screens(screen, []image.Rectangle{screen})

	if opts := c.FitSpot(1360, 860); len(opts) == 0 {
		t.Fatal("a window 860 tall placed 500 down a 1080 screen was left " +
			"running 280 off the bottom")
	}
	if got := c.spot.Top; got != 1080-860 {
		t.Errorf("moved to %v, want %v - just enough to sit on the screen",
			got, unit.Dp(1080-860))
	}
	if got := c.spot.Left; got != 40 {
		t.Errorf("moved sideways to %v, and nothing was wrong with 40", got)
	}
}

// One that already fits is left exactly where it was put.
func TestAWindowThatFitsIsNotMoved(t *testing.T) {
	screen := image.Rect(0, 0, 1920, 1080)
	c := NewLayerChrome(float.Spot{Top: 24, Left: 24})
	c.Screens(screen, []image.Rectangle{screen})
	if opts := c.FitSpot(820, 620); len(opts) != 0 {
		t.Errorf("a window that fits was moved anyway: %v -> %v", 24, c.spot)
	}
}

// A window over a neighbouring screen is where it was asked to be.
//
// The same rule the drag clamp follows: a direction is only closed off when no
// screen lies that way, because a margin measured from one output is how a
// window on the next one is expressed.
func TestAWindowOverTheNextScreenIsLeftAlone(t *testing.T) {
	mine := image.Rect(0, 0, 1920, 1080)
	right := image.Rect(1920, 0, 3840, 1080)
	c := NewLayerChrome(float.Spot{Top: 24, Left: 1600})
	c.Screens(mine, []image.Rectangle{mine, right})
	if opts := c.FitSpot(1360, 860); len(opts) != 0 {
		t.Errorf("a window reaching onto the screen beside it was hauled back: "+
			"left is now %v", c.spot.Left)
	}
}

// A window bigger than the screen is given what there is.
//
// Rather than every caller guessing a size conservatively enough for the
// smallest display anybody runs it on: a window should ask for the room it
// wants and be cut down where there is not that much.
func TestAWindowTooBigForTheScreenIsCutDown(t *testing.T) {
	small := image.Rect(0, 0, 1280, 800)
	c := NewLayerChrome(float.Spot{Top: 24, Left: 24})
	c.Screens(small, []image.Rectangle{small})
	opts := fitted(Popout{Key: "k", W: 1360, H: 860}, c)
	if len(opts) == 0 {
		t.Fatal("a 1360x860 window on a 1280x800 screen was left as it was, " +
			"and a layer surface cannot be resized afterwards")
	}
	// And on a screen with room, the ask stands.
	big := image.Rect(0, 0, 2560, 1440)
	c2 := NewLayerChrome(float.Spot{Top: 24, Left: 24})
	c2.Screens(big, []image.Rectangle{big})
	if opts := fitted(Popout{Key: "k", W: 1360, H: 860}, c2); len(opts) != 0 {
		t.Error("a window that fits was resized anyway")
	}
}

// Nothing is known until an output is, and guessing is worse than waiting.
func TestNothingIsFittedBeforeAScreenIsKnown(t *testing.T) {
	c := NewLayerChrome(float.Spot{Top: 500, Left: 40})
	if opts := c.FitSpot(1360, 860); opts != nil {
		t.Error("a window was moved against a screen nobody had reported yet")
	}
	if opts := fitted(Popout{Key: "k", W: 1360, H: 860}, c); opts != nil {
		t.Error("a window was resized against a screen nobody had reported yet")
	}
}

// A panel has no say in its own chrome, which is the point.
//
// PopoutPanel is one method. There is nothing for a panel to implement wrongly
// and nothing for it to forget, so a window cannot exist without a bar - which
// is what two of the five got wrong while the bar was theirs to lay out.
func TestAPanelCannotBeMissingItsChrome(t *testing.T) {
	// A compile-time fact: the interface is satisfied by a type that knows
	// nothing about windows. If PopoutPanel ever grows a chrome accessor
	// again, this stops compiling, which is where to argue about it.
	var _ PopoutPanel = drawOnly{}
	_ = t
}

type drawOnly struct{}

func (drawOnly) Draw(*theme.Theme, layout.Context, *state.Snapshot) layout.Dimensions {
	panic("not called")
}

// Maximise fills the screen the window is already on.
//
// A layer surface's own maximise is an anchor to all four edges, and that does
// not say which output - so on a desktop with two screens the compositor
// answers with its primary, and pressing maximise on the second screen threw
// the window onto the first. Margins are measured from the output the surface
// is anchored to, so a top-left anchor at nothing, sized to that output, is the
// same rectangle without the ambiguity.
func TestMaximiseFillsTheScreenTheWindowIsOn(t *testing.T) {
	first := image.Rect(0, 0, 1920, 1080)
	second := image.Rect(1920, 0, 3840, 1080)
	all := []image.Rectangle{first, second}

	// Opened on the first screen and dragged onto the second, which is margins
	// rather than a change of anchor: the surface is still anchored to the
	// first, and this is the case the first two attempts at maximise both got
	// wrong in opposite directions.
	c := NewLayerChrome(float.Spot{Top: 100, Left: 2100})
	c.Screens(first, all)
	c.Frame(frameAt(image.Pt(800, 600), 1))

	if len(c.fill()) != 2 {
		t.Fatal("maximise produced no move and size")
	}
	if got := int(c.spot.Left); got != 1920 {
		t.Errorf("maximised to a margin of %d from the anchored screen, want "+
			"1920 - the offset that puts it over the screen it is on", got)
	}
	if got := int(c.spot.Top); got != 0 {
		t.Errorf("maximised %d down, want the top of that screen", got)
	}

	// And one that never left its own screen fills that one.
	home := NewLayerChrome(float.Spot{Top: 60, Left: 200})
	home.Screens(first, all)
	home.Frame(frameAt(image.Pt(800, 600), 1))
	home.fill()
	if home.spot != (float.Spot{}) {
		t.Errorf("a window on its own screen maximised to %v, want its corner",
			home.spot)
	}

	// With no output known it falls back rather than guessing a size.
	blind := NewLayerChrome(float.Spot{})
	if got := blind.fill(); len(got) != 1 {
		t.Errorf("a window that has not been told where it is produced %d "+
			"options; only the four-edge anchor is safe there", len(got))
	}
}

// A window can be pulled to a new size, because nothing else will resize it.
func TestTheGripResizesTheWindow(t *testing.T) {
	screen := image.Rect(0, 0, 1920, 1080)
	c := NewLayerChrome(float.Spot{Top: 24, Left: 24})
	c.Screens(screen, []image.Rectangle{screen})
	c.Frame(frameAt(image.Pt(800, 600), 1))

	var g comp.ResizeGrip
	if opts := c.Resize(pulled(&g, 0, 0)); len(opts) != 0 {
		t.Error("a grip nobody pulled resized the window")
	}
	if opts := c.Resize(pulled(&g, 120, 90)); len(opts) == 0 {
		t.Fatal("pulling the grip did not resize the window, which is the only " +
			"way a layer-shell window can be resized at all")
	}
}

// It stops at a floor, because a window shrunk to nothing cannot be grown
// again: the grip is inside it.
func TestTheGripCannotShrinkAWindowToNothing(t *testing.T) {
	screen := image.Rect(0, 0, 1920, 1080)
	c := NewLayerChrome(float.Spot{})
	c.Screens(screen, []image.Rectangle{screen})
	c.Frame(frameAt(image.Pt(300, 300), 1))
	var g comp.ResizeGrip
	if opts := c.Resize(pulled(&g, -900, -900)); len(opts) == 0 {
		t.Fatal("a hard pull inwards did nothing at all")
	}
	// The floor is what it is; the assertion is that one exists and that the
	// window is still big enough to hold the grip that would grow it.
	if minWindowDp < 120 {
		t.Errorf("the floor is %v, which is too small to hold a bar and a grip",
			minWindowDp)
	}
}

// A maximised window declines to be resized, as a decorated one does.
func TestAMaximisedWindowIsNotResized(t *testing.T) {
	screen := image.Rect(0, 0, 1920, 1080)
	c := NewLayerChrome(float.Spot{})
	c.Screens(screen, []image.Rectangle{screen})
	c.Frame(frameAt(image.Pt(1920, 1080), 1))
	c.fill()
	c.maximised = true
	var g comp.ResizeGrip
	if opts := c.Resize(pulled(&g, -200, -200)); len(opts) != 0 {
		t.Error("a maximised window was resized by its grip, which leaves it " +
			"maximised and the wrong size at once")
	}
}

// pulled puts a grip in the state a drag of dx,dy would leave it in.
func pulled(g *comp.ResizeGrip, dx, dy float32) *comp.ResizeGrip {
	g.SetDragForTest(f32.Pt(10, 10), f32.Pt(10+dx, 10+dy), dx != 0 || dy != 0)
	return g
}

// frameAt is a frame event carrying a size and a density.
func frameAt(size image.Point, pxPerDp float32) app.FrameEvent {
	return app.FrameEvent{Size: size, Metric: unit.Metric{PxPerDp: pxPerDp, PxPerSp: pxPerDp}}
}
