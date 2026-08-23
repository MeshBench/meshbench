package workbench

import (
	"image"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// The parts a board is drawn out of: its screen, its lamps, the things
// somebody can press.
//
// One renderer per kind rather than one per board. What varies between boards
// is which of these appear and on which pins, and that is declared in the
// board's own file - so a new board adds no drawing code at all, and a part
// that draws wrongly draws wrongly everywhere at once, where it will be seen.
// screen draws the display at a whole-number scale.
//
// Whole-number because a smoothed 128x64 panel is a picture of something the
// firmware did not draw. Better to show it small and true than large and
// invented.
func (p *nodeWindowPanel) screen(t *theme.Theme, gtx layout.Context,
	panel *scenario.Panel, st *state.NodeStat) layout.Dimensions {

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

// lamps draws the board's lights.
func (p *nodeWindowPanel) lamps(t *theme.Theme, gtx layout.Context,
	panel *scenario.Panel, st *state.NodeStat) layout.Dimensions {

	parts := panel.PartsOfKind(scenario.Lamp)
	if len(parts) == 0 {
		return comp.Text(t, t.Sz.Caption, t.P.Faint, "no lamp declared")(gtx)
	}
	children := make([]layout.FlexChild, 0, len(parts)*2)
	for i := range parts {
		part := parts[i]
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.lamp(t, gtx, part, st)
		}))
		children = append(children, layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}
func (p *nodeWindowPanel) lamp(t *theme.Theme, gtx layout.Context,
	part scenario.Part, st *state.NodeStat) layout.Dimensions {

	d := gtx.Dp(unit.Dp(10))
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(d, d)
			// Whether a lamp is lit is not modelled yet: the pin is declared
			// and nothing watches it. Drawn as an outline rather than as an
			// unlit lamp, because "off" and "not modelled" are different
			// facts and the second one is about us, not the board.
			rr := d / 2
			paint.FillShape(gtx.Ops, t.P.Rule,
				clip.RRect{Rect: image.Rectangle{Max: size}, NE: rr, NW: rr, SE: rr, SW: rr}.Op(gtx.Ops))
			inner := image.Rect(1, 1, d-1, d-1)
			ir := (d - 2) / 2
			paint.FillShape(gtx.Ops, t.P.Panel,
				clip.RRect{Rect: inner, NE: ir, NW: ir, SE: ir, SW: ir}.Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		}),
		layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, part.Name)),
	)
}

// buttons draws what somebody can press, and says where a board has none.
func (p *nodeWindowPanel) buttons(t *theme.Theme, gtx layout.Context,
	panel *scenario.Panel) layout.Dimensions {

	parts := panel.PartsOfKind(scenario.Button)
	if len(parts) == 0 {
		return comp.Text(t, t.Sz.Caption, t.P.Faint, "no button declared")(gtx)
	}
	children := make([]layout.FlexChild, 0, len(parts)*2)
	for i := range parts {
		part := parts[i]
		if part.Pin == scenario.PinNone {
			// The board says it has none. Said rather than omitted: an
			// absence somebody recorded is worth more than a gap that reads
			// as nobody having looked. Not a control, because there is
			// nothing to press.
			children = append(children, layout.Rigid(
				comp.Pill(t, t.P.Faint, part.Name+" - none on this board")))
			children = append(children, layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout))
			continue
		}
		// Pooled by pin, because widget identity is address: rebuilding one
		// per frame loses the press half way through.
		btn := p.buttonFor(part.Pin)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			col := t.P.Dim
			if btn.Pressed() {
				col = t.P.Accent
			}
			return btn.Layout(gtx, comp.Pill(t, col, part.Name))
		}))
		children = append(children, layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

// ball draws a trackball as the four ways it can be rolled, and its click
// where the board declares one.
//
// Arrows rather than a drawn ball, because what reaches the firmware is not a
// position: each direction is a plain line that changes level as the ball
// turns past it, and the firmware counts the changes. One press is one step,
// which is exactly what these produce.
func (p *nodeWindowPanel) ball(t *theme.Theme, gtx layout.Context,
	panel *scenario.Panel) layout.Dimensions {

	parts := panel.PartsOfKind(scenario.Ball)
	if len(parts) == 0 {
		return layout.Dimensions{}
	}
	part := parts[0]
	// Named in the order a board declares them, and only as many as it has:
	// a trackball with fewer lines than four is a declaration to fix rather
	// than a thing to draw four arrows for.
	dirs := [4]struct {
		way   comp.Arrow
		label string
	}{
		{comp.ArrowUp, "up"}, {comp.ArrowDown, "down"},
		{comp.ArrowLeft, "left"}, {comp.ArrowRight, "right"},
	}
	if len(part.Pins) > len(dirs) {
		part.Pins = part.Pins[:len(dirs)]
	}
	children := make([]layout.FlexChild, 0, len(part.Pins)*2+1)
	children = append(children, layout.Rigid(
		comp.Text(t, t.Sz.Caption, t.P.Dim, part.Name)))
	children = append(children, layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout))
	for i, pin := range part.Pins {
		if pin == scenario.PinNone {
			continue
		}
		dir := dirs[i]
		btn := p.buttonFor(pin)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			col := t.P.Dim
			if btn.Pressed() {
				col = t.P.Accent
			}
			return btn.Layout(gtx, comp.ArrowPill(t, col, dir.way, dir.label))
		}))
		children = append(children, layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}
