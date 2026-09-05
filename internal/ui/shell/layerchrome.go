// What a window with a drawn title bar does about its bar: the machinery
// under comp.TitleBar's glyphs, shared by the pop-out windows and the node
// windows because they chrome themselves identically.
//
// The layer-shell protocol is what makes this necessary and what constrains
// it: a layer surface has no decoration, so the bar exists at all only
// because this file's window draws one; it is placed by margins, so a drag
// is a re-placement per pointer move; maximise is anchoring to all four
// edges; and there is no minimise at all - the close glyph returns the
// panel to the main window, which is what minimising a panel means.
package shell

import (
	"fmt"
	"image"
	"os"

	"gioui.org/f32"

	"gioui.org/app"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/diag"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/float"
)

// barKeepDp is how much of the window has to stay on the output for the bar
// to remain grabbable: its height vertically, and enough width horizontally
// to get a hold of. Window-management geometry rather than interface
// drawing, which is why it is a constant here and not a theme token.
const (
	barHeightDp = 40
	barGrabDp   = 120
)

// KeepAbove is the machine preference for whether the next window stays
// above the main one, as the store last published it. Default on, because
// the issue it answers is a panel lost behind the main window.
func KeepAbove(st *state.Store) bool {
	if s := st.Snapshot(); s != nil {
		return s.KeepAbove
	}
	return true
}

// LayerChrome tracks where a layer window is and whether it is maximised,
// and turns the bar's drags and glyph presses into window options.
type LayerChrome struct {
	spot      float.Spot
	maximised bool
	// restore is what maximise took: the place, and the size to give back.
	restore struct {
		spot float.Spot
		w, h unit.Dp
	}
	// size, pxPerDp and outputs are the last frame's facts: the first two
	// so a drag or a restore can be stated in the units the options take,
	// and the screens so a drag cannot carry the window somewhere its bar
	// can no longer be reached from.
	size    image.Point
	pxPerDp float32
	// screen is the output the margins are measured from, and outputs is
	// every one of them - both, because the first says where the window is
	// and the second says whether there is anywhere to go.
	screen  image.Rectangle
	outputs []image.Rectangle
	// dragging and grabbedAt hold the point the bar was taken hold of, in
	// logical pixels rather than in the window's own - because the window's
	// own change underneath a drag that crosses to a screen at another scale.
	dragging  bool
	grabbedAt f32.Point
}

// NewLayerChrome is a chrome for a window placed at spot.
func NewLayerChrome(spot float.Spot) *LayerChrome {
	return &LayerChrome{spot: spot, pxPerDp: 1}
}

// frame notes the window's own facts for the next update.
func (c *LayerChrome) Frame(e app.FrameEvent) {
	c.size, c.pxPerDp = e.Size, e.Metric.PxPerDp
}

// Screens notes where the outputs are and which one the margins are measured
// from, as the fork reports them in the same pixels as the frame size.
//
// The anchor is kept once it is known, and a later empty report is ignored.
// That is not tidiness: a layer surface's margins are measured from the output
// it was created on for its whole life, but the compositor stops listing the
// surface against that output the moment it has been dragged entirely onto
// another one - and the fork reports the anchor by looking the surface up in
// the outputs it is currently on, so the anchor comes back as an empty
// rectangle exactly when the window is somewhere else.
//
// Everything here keys off that rectangle, so the window went unclamped, unfit
// and unmaximisable precisely when it was on the second screen. Measured with
// scratch/layerprobe against a real compositor after three guesses at what the
// protocol did:
//
//	anchor=(0,0)-(2560,1440)  outputs=[(0,0)-(2560,1440) (2560,0)-(4480,1080)]
//	... moved onto the output at (2560,0) by margins ...
//	anchor=(0,0)-(0,0)
//
// An empty anchor is the report being unavailable, never the anchor changing:
// the fork holds the surface's own output from the first one it entered and
// keeps it.
func (c *LayerChrome) Screens(mine image.Rectangle, all []image.Rectangle) {
	if !mine.Empty() {
		c.screen = mine
	}
	c.outputs = all
}

// Maximise is fill, for a caller outside this package: the scratch probe that
// measures this against a real compositor, which is how the rule it follows was
// established rather than guessed at a fourth time.
func (c *LayerChrome) Maximise() []app.Option { return c.fill() }

// PlaceAt records where something else has just put the window, so the chrome's
// idea of the margins is the compositor's. Only the probe needs it; the window
// loop moves the window through this type.
func (c *LayerChrome) PlaceAt(spot float.Spot) { c.spot = spot }

// Maximised reports whether the window is anchored to all four edges, so the
// bar draws the right glyph.
func (c *LayerChrome) Maximised() bool { return c.maximised }

// Screen is the output the margins are measured from, empty until one is
// known.
func (c *LayerChrome) Screen() image.Rectangle { return c.screen }

// FitSpot moves the window so a window of this size opens wholly on screen.
//
// The cascade that places a new window knows nothing about how big it is, so a
// tall one placed a little way down the screen runs off the bottom - and on a
// layer surface what runs off cannot be dragged back into view by the
// compositor, only by our own bar, which is itself off the screen by then.
//
// Only ever moves it towards the top-left, and only where there is no screen
// that way: a window placed over a neighbouring output is somewhere it was
// asked to be.
func (c *LayerChrome) FitSpot(w, h unit.Dp) []app.Option {
	if c.screen.Empty() {
		return nil
	}
	was := c.spot
	if !c.neighbour(1, 0) {
		if over := c.spot.Left + w - unit.Dp(c.screen.Dx()); over > 0 {
			c.spot.Left -= over
		}
	}
	if !c.neighbour(0, 1) {
		if over := c.spot.Top + h - unit.Dp(c.screen.Dy()); over > 0 {
			c.spot.Top -= over
		}
	}
	if c.spot.Left < 0 && !c.neighbour(-1, 0) {
		c.spot.Left = 0
	}
	if c.spot.Top < 0 && !c.neighbour(0, -1) {
		c.spot.Top = 0
	}
	if c.spot == was {
		return nil
	}
	c.restore.spot = c.spot
	return []app.Option{float.Move(c.spot)}
}

// update reads the bar after it has been laid out - its glyphs collect
// their own clicks during Layout - and returns the options to apply, and
// whether the window was asked to close.
func (c *LayerChrome) Update(bar *comp.TitleBar) (opts []app.Option, close bool) {
	// Asked once: Drag reports whether an event has arrived since the last ask
	// and clears that as it answers, so a second call always says no.
	grab, pos, held, fresh := bar.Drag()
	if !held || c.maximised {
		// The hold has ended, so the next press takes a fresh anchor rather
		// than measuring from the last drag's.
		c.dragging = false
	}
	if held && fresh && !c.maximised {
		// The whole drag is done in logical pixels - the units a margin is
		// measured in - and not in the window's own pixels, because those
		// change during the drag.
		//
		// Gio takes the largest scale of the outputs a surface is on, so the
		// moment a window touches a screen at 200% its pixels-per-dp doubles.
		// The bar reports its grab in the pixels of the frame it was taken in,
		// so from then on the grab is measured in one unit and the pointer in
		// another, and the difference between them is nonsense: the window
		// leaps, the leap changes which screens it is touching, the scale
		// changes back, and it leaps again. That is the bouncing at a
		// boundary between two screens of different scale.
		//
		// So the grab is converted once, at the press, and every position
		// after it with whatever the metric is at the time. Both are then
		// logical pixels within the surface however the scale changed in
		// between, and the shape of the sum is the one it always was: each
		// event adds the whole remaining distance from the grabbed point,
		// which shrinks to nothing as the window arrives under the pointer.
		// Per-event deltas were tried and accelerated, because each one
		// counted a move that had not landed yet.
		here := f32.Pt(pos.X*c.pxToDp(), pos.Y*c.pxToDp())
		if !c.dragging {
			c.dragging = true
			c.grabbedAt = f32.Pt(grab.X*c.pxToDp(), grab.Y*c.pxToDp())
		}
		c.spot.Left += unit.Dp(here.X - c.grabbedAt.X)
		c.spot.Top += unit.Dp(here.Y - c.grabbedAt.Y)
		c.clamp()
		opts = append(opts, float.Move(c.spot))
	}
	if bar.MaximiseClicked() {
		if c.maximised {
			c.spot, c.maximised = c.restore.spot, false
			opts = append(opts, float.Move(c.spot),
				app.Size(c.restore.w, c.restore.h))
		} else {
			c.restore.spot, c.maximised = c.spot, true
			c.restore.w, c.restore.h = c.pxToDpSize(c.size)
			opts = append(opts, c.fill()...)
		}
		bar.Maximised = c.maximised
	}
	return opts, bar.CloseClicked()
}

// Resize turns a pull on the corner grip into a new size.
//
// The same arithmetic as the drag above and for the same reason: the target is
// the current size plus the whole remaining distance from the grabbed point,
// recomputed every event, because a resize is in flight while the events pause
// and a delta taken across one counts it twice.
//
// A maximised window declines, as a decorated one does. Below the floor it
// stops, because a window shrunk to nothing cannot be grown again - the grip is
// inside it. Above the screen it stops too, for the reason a window is fitted
// when it opens.
func (c *LayerChrome) Resize(g *comp.ResizeGrip) []app.Option {
	grab, pos, held, fresh := g.Drag()
	if !held || !fresh || c.maximised || c.pxPerDp <= 0 {
		return nil
	}
	w, h := c.pxToDpSize(c.size)
	w += unit.Dp((pos.X - grab.X) * c.pxToDp())
	h += unit.Dp((pos.Y - grab.Y) * c.pxToDp())
	if w < minWindowDp {
		w = minWindowDp
	}
	if h < minWindowDp {
		h = minWindowDp
	}
	if !c.screen.Empty() {
		if max := unit.Dp(c.screen.Dx()); w > max {
			w = max
		}
		if max := unit.Dp(c.screen.Dy()); h > max {
			h = max
		}
	}
	return []app.Option{app.Size(w, h)}
}

// minWindowDp is the smallest a window may be pulled to: enough to hold its
// own bar and grip, so it can always be grown again.
const minWindowDp = unit.Dp(240)

// fill is maximise: the window over the whole of the screen it is on now.
//
// Not the four-edge anchor, which is what a layer surface's own maximise is.
// That fills the output the surface was created on, whatever screen it has
// since been carried to - so maximise on the second screen threw the window
// back to the first, which is the fault this started as.
//
// Margins are measured from the anchored output for the surface's whole life,
// so where the window is now is that output's corner plus the margins, and
// which screen that lands on is an ordinary question about rectangles. It only
// became a hard one because the anchor is reported empty once the window has
// left it - see Screens, which is where that is now held onto.
func (c *LayerChrome) fill() []app.Option {
	if c.screen.Empty() {
		// Nothing has ever been reported, so there is no arithmetic to do and
		// the compositor's own answer is the only one available.
		return []app.Option{float.Maximise()}
	}
	on := c.screenUnder(c.centre())
	c.spot = float.Spot{
		Left: unit.Dp(on.Min.X - c.screen.Min.X),
		Top:  unit.Dp(on.Min.Y - c.screen.Min.Y),
	}
	layerLog("maximise: anchor=%v centre=%v onto=%v margins=(%v,%v) size=%vx%v",
		c.screen, c.centre(), on, c.spot.Left, c.spot.Top, on.Dx(), on.Dy())
	return []app.Option{
		float.Move(c.spot),
		app.Size(unit.Dp(on.Dx()), unit.Dp(on.Dy())),
	}
}

// screenUnder is the output a point is on, or the anchored one when the point
// is in a gap between screens or on none of them.
func (c *LayerChrome) screenUnder(at image.Point) image.Rectangle {
	for _, o := range c.outputs {
		if at.In(o) {
			return o
		}
	}
	return c.screen
}

// centre is where the middle of the window is on the desktop.
//
// In the coordinates the outputs are given in: the anchored output's own
// corner, plus the margins, plus half the window. The size is the window's
// pixels and the rest is the desktop's logical units, so the size is converted
// - the same distinction the drag clamp makes, and the one that halved a bound
// on every screen above 100% when it was missed.
func (c *LayerChrome) centre() image.Point {
	w, h := c.pxToDpSize(c.size)
	return image.Pt(
		c.screen.Min.X+int(c.spot.Left)+int(w)/2,
		c.screen.Min.Y+int(c.spot.Top)+int(h)/2,
	)
}

// recall places the window at spot: the escape hatch for one that has ended
// up somewhere its bar cannot be reached from. Raising means nothing to a
// layer surface - the compositor stacks the layer, not the windows in it -
// so this is what the raise wish becomes for a layered window.
func (c *LayerChrome) Recall(spot float.Spot) []app.Option {
	c.spot, c.restore.spot = spot, spot
	opts := []app.Option{float.Move(spot)}
	if c.maximised {
		// Un-maximised on the way back, and this is the case that matters
		// most: a maximised window is the one whose bar cannot be reached,
		// because maximise is what put it on a screen the pointer is not on.
		// This used to return nothing at all for one - "there is nothing to
		// bring back" - so the one window that could not be recovered was the
		// one that needed recovering, and closing the application was the only
		// way out of it.
		c.maximised = false
		w, h := c.restore.w, c.restore.h
		if w < minWindowDp || h < minWindowDp {
			w, h = defaultRecallDp, defaultRecallDp
		}
		opts = append(opts, app.Size(w, h))
	}
	return opts
}

// defaultRecallDp is the size a recalled window is given when nothing sensible
// was remembered - a window maximised before it had ever been measured.
const defaultRecallDp = unit.Dp(800)

// clamp keeps the bar somewhere it can be grabbed.
//
// A margin is measured from the screen the surface is anchored to, so a
// negative one means "left of this screen" - which is off the desktop if this
// screen is the leftmost, and perfectly ordinary if there is another one
// there. That is the whole of the bug this replaces: clamping margins at zero
// forbade the second case along with the first, and clamping them at the
// screen's width undid a move the moment the compositor handed the surface
// over.
//
// So a direction is only closed off when no screen lies that way. Whether the
// window then ends up in a gap between screens is not something margins can
// express, and recall is the way back from one.
func (c *LayerChrome) clamp() {
	if c.screen.Empty() || c.pxPerDp <= 0 {
		layerLog("clamp skipped: screen=%v pxPerDp=%v", c.screen, c.pxPerDp)
		return
	}
	was := c.spot
	defer func() {
		layerLog("screen=%v outputs=%v neighbours L=%v R=%v U=%v D=%v spot %v,%v -> %v,%v",
			c.screen, c.outputs,
			c.neighbour(-1, 0), c.neighbour(1, 0), c.neighbour(0, -1), c.neighbour(0, 1),
			was.Left, was.Top, c.spot.Left, c.spot.Top)
	}()
	if !c.neighbour(-1, 0) && c.spot.Left < 0 {
		c.spot.Left = 0
	}
	if !c.neighbour(0, -1) && c.spot.Top < 0 {
		c.spot.Top = 0
	}
	// The screen's own units, not the window's. A margin and an output's
	// logical rectangle are both in the coordinates the compositor lays the
	// desktop out in, and dividing one of them by the window's pixels-per-dp
	// halved the bound on any screen above 100%.
	if !c.neighbour(1, 0) {
		if max := unit.Dp(c.screen.Dx()) - barGrabDp; c.spot.Left > max {
			c.spot.Left = max
		}
	}
	if !c.neighbour(0, 1) {
		if max := unit.Dp(c.screen.Dy()) - barHeightDp; c.spot.Top > max {
			c.spot.Top = max
		}
	}
}

// neighbour reports whether a screen lies in the given direction from the one
// the margins are measured from, sharing some of its span across.
//
// Sharing the span, because a screen diagonally opposite is not somewhere a
// horizontal drag arrives: two side by side and one below the right-hand of
// them means there is nothing to the left of the left-hand one, whatever the
// bounding box says.
func (c *LayerChrome) neighbour(dx, dy int) bool {
	for _, o := range c.outputs {
		if o == c.screen {
			continue
		}
		switch {
		case dx < 0 && o.Max.X <= c.screen.Min.X && spans(o.Min.Y, o.Max.Y, c.screen.Min.Y, c.screen.Max.Y):
			return true
		case dx > 0 && o.Min.X >= c.screen.Max.X && spans(o.Min.Y, o.Max.Y, c.screen.Min.Y, c.screen.Max.Y):
			return true
		case dy < 0 && o.Max.Y <= c.screen.Min.Y && spans(o.Min.X, o.Max.X, c.screen.Min.X, c.screen.Max.X):
			return true
		case dy > 0 && o.Min.Y >= c.screen.Max.Y && spans(o.Min.X, o.Max.X, c.screen.Min.X, c.screen.Max.X):
			return true
		}
	}
	return false
}

// spans reports whether two ranges overlap at all.
func spans(a0, a1, b0, b1 int) bool { return a0 < b1 && b0 < a1 }

// pxToDp is the frame's pixel-per-dp as a multiplier the other way, so a
// drag measured in pixels can be added to a place stated in dp.
func (c *LayerChrome) pxToDp() float32 {
	if c.pxPerDp <= 0 {
		return 1
	}
	return 1 / c.pxPerDp
}

// pxToDpSize converts a window size in pixels to the dp the Size option
// takes, so a restore gives back the window it took.
func (c *LayerChrome) pxToDpSize(sz image.Point) (unit.Dp, unit.Dp) {
	return unit.Dp(float32(sz.X) * c.pxToDp()), unit.Dp(float32(sz.Y) * c.pxToDp())
}

// layerLog says what a drag is being measured against, when asked.
//
// Diagnostic rather than logging: a layer surface cannot be asked where it is,
// so when a drag stops at a screen edge the only way to tell a client that
// refused the move from a compositor that ignored it is to print what the
// client believed and what it then sent.
//
//	MESHBENCH_LOG=layer go run ./cmd/meshbench workbench
//
// MESHBENCH_LAYER_DEBUG still turns it on, so a script or a note that named the
// old variable keeps working; MESHBENCH_LOG is the one that also selects the
// other domains at the same time.
func layerLog(format string, args ...any) {
	if !diag.On("layer") && os.Getenv("MESHBENCH_LAYER_DEBUG") == "" {
		return
	}
	// A drag reports several times a frame and says the same thing each time
	// until something moves, so only changes are printed - what is wanted here
	// is the moment a value stops changing, and that is unreadable inside a
	// thousand identical lines.
	line := fmt.Sprintf(format, args...)
	if line == lastLayerLog {
		return
	}
	lastLayerLog = line
	fmt.Fprintln(os.Stderr, "layer: "+line)
}

var lastLayerLog string
