package nodeview

import (
	"image"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/MeshBench/meshbench/internal/app/state"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// The board's screen, drawn at a whole-number scale.
//
// Its lamps, buttons and trackball are comp.BoardControls, because the bring-up
// window draws the same parts and a second copy would be a second set of
// answers to "which pin is up" - which took working out from two frames that
// disagree.
//
// screen draws the display at a whole-number scale.
//
// Whole-number because a smoothed 128x64 panel is a picture of something the
// firmware did not draw. Better to show it small and true than large and
// invented.
func (p *WindowPanel) screen(t *theme.Theme, gtx layout.Context,
	panel *hw.Panel, st *state.NodeStat) layout.Dimensions {

	if panel.Screen == nil {
		return comp.Text(t, t.Sz.Caption, t.P.Faint, "this board has no display")(gtx)
	}
	sc := panel.Screen
	scale := gtx.Constraints.Max.X / sc.WidthPx
	if scale < 1 {
		scale = 1
	}
	if scale > 4 {
		scale = 4
	}
	w, h := sc.WidthPx*scale, sc.HeightPx*scale
	p.screenScale = scale

	var note string
	switch {
	case st == nil || !st.Running:
		note = "not powered"
	case st.Screen == nil:
		note = "the firmware has not drawn anything yet"
	case !st.Screen.On:
		// Not a fault. MeshCore switches the panel off after an idle and the
		// board's own button brings it back, so a dark screen here is the
		// firmware doing what it should.
		note = "asleep - the firmware switched it off"
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(w, h)
			// A panel with a touch layer takes pointer events, in the panel's
			// own pixels rather than the window's - the firmware is told where
			// on its screen a finger is, and the scale is ours not its.
			// The panel takes pointer events where it has a touch layer, and
			// key events where the board has a keyboard - both addressed to
			// the drawn screen, because that is the board somebody is
			// pointing at.
			if p.touchable(panel) || p.typeable(panel) {
				defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
				if p.touchable(panel) {
					event.Op(gtx.Ops, &p.screenTouch)
				}
				if p.typeable(panel) {
					event.Op(gtx.Ops, &p.screenKeys)
				}
			}
			// The panel's own colours, not the theme's: a display is a
			// physical object and reads as light on black whichever way the
			// interface is set.
			paint.FillShape(gtx.Ops, t.P.ScreenGround,
				clip.Rect{Max: size}.Op())
			if st != nil && st.Screen != nil && st.Screen.On {
				for y := 0; y < sc.HeightPx; y++ {
					for x := 0; x < sc.WidthPx; x++ {
						col, ok := screenPixel(t, st.Screen, x, y)
						if !ok {
							continue
						}
						r := image.Rect(x*scale, y*scale, (x+1)*scale, (y+1)*scale)
						paint.FillShape(gtx.Ops, col, clip.Rect(r).Op())
					}
				}
			}
			return layout.Dimensions{Size: size}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if note == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: t.Sp.XS}.Layout(gtx,
				comp.Text(t, t.Sz.Caption, t.P.Faint, note))
		}),
	)
}

// screenPixel is what to paint at one pixel, and whether to paint at all.
//
// A monochrome panel lights pixels in the one colour the part can produce; a
// colour one carries its own, and drawing those in a theme colour would be
// inventing a picture the firmware did not send.
func screenPixel(t *theme.Theme, sc *state.Screen, x, y int) (color.NRGBA, bool) {
	if sc.BPP == 16 {
		r, g, b, ok := sc.At(x, y)
		if !ok || (r == 0 && g == 0 && b == 0) {
			return color.NRGBA{}, false
		}
		return color.NRGBA{R: r, G: g, B: b, A: 0xff}, true
	}
	if !sc.Lit(x, y) {
		return color.NRGBA{}, false
	}
	return t.P.ScreenLit, true
}
