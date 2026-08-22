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
	// size and pxPerDp are the last frame's, so a drag or a restore can be
	// stated in the units the options take.
	size    image.Point
	pxPerDp float32
}

// newLayerChrome is a chrome for a window placed at spot.
func newLayerChrome(spot float.Spot) *layerChrome {
	return &layerChrome{spot: spot, pxPerDp: 1}
}

// frame notes the window's own facts for the next update.
func (c *layerChrome) frame(e app.FrameEvent) {
	c.size, c.pxPerDp = e.Size, e.Metric.PxPerDp
}

// update reads the bar after it has been laid out - its glyphs collect
// their own clicks during Layout - and returns the options to apply, and
// whether the window was asked to close.
func (c *layerChrome) update(bar *comp.TitleBar) (opts []app.Option, close bool) {
	if d := bar.Drag(); !c.maximised && (d.X != 0 || d.Y != 0) {
		// Margins position the window, so moving it is moving them. Clamped
		// at the output's top-left corner: a negative margin is a window
		// nobody can reach the bar of, and nothing can drag it back.
		pxToDp := float32(1)
		if c.pxPerDp > 0 {
			pxToDp = 1 / c.pxPerDp
		}
		c.spot.Top += unit.Dp(float32(d.Y) * pxToDp)
		c.spot.Left += unit.Dp(float32(d.X) * pxToDp)
		if c.spot.Top < 0 {
			c.spot.Top = 0
		}
		if c.spot.Left < 0 {
			c.spot.Left = 0
		}
		opts = append(opts, float.Move(c.spot))
	}
	if bar.MaximiseClicked() {
		if c.maximised {
			c.spot, c.maximised = c.restore.spot, false
			opts = append(opts, float.Move(c.spot),
				app.Size(c.restore.w, c.restore.h))
		} else {
			c.restore.spot, c.maximised = c.spot, true
			c.restore.w, c.restore.h = c.pxToDp(c.size)
			opts = append(opts, float.Maximise())
		}
		bar.Maximised = c.maximised
	}
	return opts, bar.CloseClicked()
}

// pxToDp converts a window size in pixels to the dp the Size option takes,
// so a restore gives back the window it took.
func (c *layerChrome) pxToDp(sz image.Point) (unit.Dp, unit.Dp) {
	if c.pxPerDp <= 0 {
		return unit.Dp(sz.X), unit.Dp(sz.Y)
	}
	return unit.Dp(float32(sz.X) / c.pxPerDp), unit.Dp(float32(sz.Y) / c.pxPerDp)
}
