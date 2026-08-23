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
package workbench

import (
	"image"

	"gioui.org/app"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
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

// keepAbove is the machine preference for whether the next window stays
// above the main one, as the store last published it. Default on, because
// the issue it answers is a panel lost behind the main window.
func keepAbove(st *state.Store) bool {
	if s := st.Snapshot(); s != nil {
		return s.KeepAbove
	}
	return true
}

// layerChrome tracks where a layer window is and whether it is maximised,
// and turns the bar's drags and glyph presses into window options.
type layerChrome struct {
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
}

// newLayerChrome is a chrome for a window placed at spot.
func newLayerChrome(spot float.Spot) *layerChrome {
	return &layerChrome{spot: spot, pxPerDp: 1}
}

// frame notes the window's own facts for the next update.
func (c *layerChrome) frame(e app.FrameEvent) {
	c.size, c.pxPerDp = e.Size, e.Metric.PxPerDp
}

// screens notes where the outputs are and which one the margins are measured
// from, as the fork reports them in the same pixels as the frame size. Empty
// means unknown, and clamping waits for it.
func (c *layerChrome) screens(mine image.Rectangle, all []image.Rectangle) {
	c.screen, c.outputs = mine, all
}

// update reads the bar after it has been laid out - its glyphs collect
// their own clicks during Layout - and returns the options to apply, and
// whether the window was asked to close.
func (c *layerChrome) update(bar *comp.TitleBar) (opts []app.Option, close bool) {
	if grab, pos, held, fresh := bar.Drag(); held && fresh && !c.maximised {
		// The grab anchor: each event's whole distance from the point the
		// bar was grabbed at is added to the place, and nothing is applied
		// without an event. Measuring from the fixed anchor, rather than
		// accumulating per-event deltas, is what keeps a drag stable while
		// the window's own move lags the pointer: a position reported
		// against the window's latest place telescopes the formula to
		// exactly right, and a position reported against an older one is
		// corrected by the very next event - while a window tracking the
		// pointer perfectly reports the same position on every event, and
		// each event's full grab-distance is exactly the movement still
		// owed. Per-event deltas instead double-counted every pending move,
		// which was a drag that accelerated until the window left the
		// screen.
		c.spot.Top += unit.Dp((pos.Y - grab.Y) * c.pxToDp())
		c.spot.Left += unit.Dp((pos.X - grab.X) * c.pxToDp())
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
			opts = append(opts, float.Maximise())
		}
		bar.Maximised = c.maximised
	}
	return opts, bar.CloseClicked()
}

// recall places the window at spot: the escape hatch for one that has ended
// up somewhere its bar cannot be reached from. Raising means nothing to a
// layer surface - the compositor stacks the layer, not the windows in it -
// so this is what the raise wish becomes for a layered window.
func (c *layerChrome) recall(spot float.Spot) []app.Option {
	if c.maximised {
		// Full-output and on top already; there is nothing to bring back.
		return nil
	}
	c.spot, c.restore.spot = spot, spot
	return []app.Option{float.Move(spot)}
}

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
func (c *layerChrome) clamp() {
	if c.screen.Empty() || c.pxPerDp <= 0 {
		return
	}
	if !c.neighbour(-1, 0) && c.spot.Left < 0 {
		c.spot.Left = 0
	}
	if !c.neighbour(0, -1) && c.spot.Top < 0 {
		c.spot.Top = 0
	}
	if !c.neighbour(1, 0) {
		if max := c.dp(c.screen.Dx()) - barGrabDp; c.spot.Left > max {
			c.spot.Left = max
		}
	}
	if !c.neighbour(0, 1) {
		if max := c.dp(c.screen.Dy()) - barHeightDp; c.spot.Top > max {
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
func (c *layerChrome) neighbour(dx, dy int) bool {
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

// dp turns a length in the pixels the screens are measured in into the dp a
// Spot is measured in.
func (c *layerChrome) dp(px int) unit.Dp { return unit.Dp(float32(px) * c.pxToDp()) }

// pxToDp is the frame's pixel-per-dp as a multiplier the other way, so a
// drag measured in pixels can be added to a place stated in dp.
func (c *layerChrome) pxToDp() float32 {
	if c.pxPerDp <= 0 {
		return 1
	}
	return 1 / c.pxPerDp
}

// pxToDpSize converts a window size in pixels to the dp the Size option
// takes, so a restore gives back the window it took.
func (c *layerChrome) pxToDpSize(sz image.Point) (unit.Dp, unit.Dp) {
	return unit.Dp(float32(sz.X) * c.pxToDp()), unit.Dp(float32(sz.Y) * c.pxToDp())
}
