package workbench

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// The Hardware tab: this board drawn as itself.
//
// Nothing here is written per board. A board's profile says what it has - a
// screen of a given size, a lamp on a pin, a button that reads low when it is
// held - and this draws whatever it said. Adding a lamp to a board is a line
// in that board's file; there is no matching change here, which is the point:
// two dozen hand-written panels is where tedium turns into error.
//
// Two halves. The board on the left, at the proportions it really has, so it
// can be recognised. The same things as facts on the right, in mono, because
// those get compared down a column.

// hardware draws the tab.
func (p *nodeWindowPanel) hardware(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	st := p.statFor(s)
	panel := p.boardPanel(s)

	if panel == nil {
		// Not an error and not an empty frame: nobody has established what
		// this board carries, and saying so is different from saying it
		// carries nothing.
		return comp.Inset(t, t.Sp.M, comp.Text(t, t.Sz.Body, t.P.Dim,
			"Nothing is recorded about what this board shows or what can be "+
				"pressed on it."))(gtx)
	}

	return comp.Inset(t, t.Sp.M, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start,
			Spacing: layout.SpaceEnd}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.device(t, gtx, panel, st)
			}),
			layout.Rigid(layout.Spacer{Width: t.Sp.L}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return p.hardwareFacts(t, gtx, panel, st)
			}),
		)
	})(gtx)
}

// boardPanel is what this node's board declares, or nil where nothing is
// recorded. The board decides; there is no setting that could disagree with it.
func (p *nodeWindowPanel) boardPanel(s *state.Snapshot) *scenario.Panel {
	st := p.statFor(s)
	if st == nil || st.Board == "" {
		return nil
	}
	b, err := scenario.BoardByName(st.Board)
	if err != nil {
		return nil
	}
	return b.Hardware
}

// device is the board: lamps above, screen in the middle, buttons below.
func (p *nodeWindowPanel) device(t *theme.Theme, gtx layout.Context,
	panel *scenario.Panel, st *state.NodeStat) layout.Dimensions {

	// Hug the board rather than fill the pane. A card stretched to the height
	// of the window puts the thing being looked at in a corner of a lot of
	// nothing, and makes a small board look like a broken layout.
	gtx.Constraints.Max.X = gtx.Dp(unit.Dp(420))
	gtx.Constraints.Min = image.Point{}
	return comp.Card(t, "", func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.lamps(t, gtx, panel, st)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.screen(t, gtx, panel, st)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.buttons(t, gtx, panel)
			}),
		)
	})(gtx)
}

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
			if p.touchable(panel) {
				defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
				event.Op(gtx.Ops, &p.screenTouch)
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

// touchable reports whether this board has a panel worth aiming at.
func (p *nodeWindowPanel) touchable(panel *scenario.Panel) bool {
	return panel != nil && panel.Screen != nil &&
		len(panel.PartsOfKind(scenario.Touch)) > 0
}

// boardTouches turns pointer events on the drawn screen into touches at the
// panel's own coordinates.
//
// Scaled back rather than sent as drawn: the firmware knows its panel is 320
// across and nothing else, and sending window pixels would make every touch
// land wherever the interface happened to be sized.
func (p *nodeWindowPanel) boardTouches(gtx layout.Context, s *state.Snapshot) {
	panel := p.boardPanel(s)
	if !p.touchable(panel) || p.OnDo == nil {
		return
	}
	sc := panel.Screen
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &p.screenTouch,
			Kinds:  pointer.Press | pointer.Release | pointer.Drag,
		})
		if !ok {
			break
		}
		pe, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		if p.screenScale <= 0 {
			continue
		}
		x := int(pe.Position.X) / p.screenScale
		y := int(pe.Position.Y) / p.screenScale
		if x < 0 || y < 0 || x >= sc.WidthPx || y >= sc.HeightPx {
			continue
		}
		p.OnDo("board.touch", map[string]any{
			"node": p.node, "x": x, "y": y,
			"down": pe.Kind != pointer.Release,
		})
	}
}

// buttonFor is this pin's control, kept across frames.
func (p *nodeWindowPanel) buttonFor(pin int) *widget.Clickable {
	if p.boardButtons == nil {
		p.boardButtons = map[int]*widget.Clickable{}
	}
	if b, ok := p.boardButtons[pin]; ok {
		return b
	}
	b := &widget.Clickable{}
	p.boardButtons[pin] = b
	return b
}

// boardPresses turns what the pointer did into holds and releases.
//
// Held rather than clicked, because the firmware behind these pins cares: a
// press wakes a sleeping display and a long one powers the board off, and a
// control that only ever produced a tap could reach neither.
func (p *nodeWindowPanel) boardPresses(gtx layout.Context, s *state.Snapshot) {
	panel := p.boardPanel(s)
	if panel == nil || p.OnDo == nil {
		return
	}
	for _, part := range panel.PartsOfKind(scenario.Button) {
		if part.Pin == scenario.PinNone {
			continue
		}
		btn := p.buttonFor(part.Pin)
		down := btn.Pressed()
		if down == p.buttonDown[part.Pin] {
			continue
		}
		if p.buttonDown == nil {
			p.buttonDown = map[int]bool{}
		}
		p.buttonDown[part.Pin] = down
		p.OnDo("board.press", map[string]any{
			"node": p.node, "pin": part.Pin, "down": down})
	}
}

// hardwareFacts is the same things as a column of readable values.
func (p *nodeWindowPanel) hardwareFacts(t *theme.Theme, gtx layout.Context,
	panel *scenario.Panel, st *state.NodeStat) layout.Dimensions {

	type row struct{ what, val string }
	var rows []row
	if s := panel.Screen; s != nil {
		rows = append(rows, row{"screen",
			fmt.Sprintf("%s %dx%d", s.Controller, s.WidthPx, s.HeightPx)})
		switch s.Bus {
		case scenario.BusI2C:
			rows = append(rows, row{"", fmt.Sprintf("I2C 0x%02X", s.Addr)})
		case scenario.BusSPI:
			rows = append(rows, row{"", fmt.Sprintf("SPI cs %d dc %d", s.CS, s.DC)})
		}
		var showing string
		switch {
		case st == nil || !st.Running:
			showing = "not powered"
		case st.Screen == nil:
			showing = "not drawn yet"
		case st.Screen.On:
			showing = "on"
		default:
			showing = "off"
		}
		rows = append(rows, row{"", showing})
	} else {
		rows = append(rows, row{"screen", "none"})
	}
	for _, part := range panel.Parts {
		if part.Kind == scenario.Lamp || part.Kind == scenario.Button {
			where := fmt.Sprintf("pin %d", part.Pin)
			if part.Pin == scenario.PinNone {
				where = "none"
			}
			rows = append(rows, row{part.Kind.String() + " " + part.Name, where})
			continue
		}
		rows = append(rows, row{part.Kind.String(), part.Name})
	}
	// Said once, on the tab rather than in a tooltip: this is what the
	// firmware drew, not what the panel looks like.
	children := make([]layout.FlexChild, 0, len(rows)+2)
	for i := range rows {
		r := rows[i]
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(unit.Dp(120))
					return comp.Text(t, t.Sz.Caption, t.P.Dim, r.what)(gtx)
				}),
				layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Ink, r.val)),
			)
		}))
	}
	children = append(children, layout.Rigid(layout.Spacer{Height: t.Sp.M}.Layout))
	children = append(children, layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
		"What the firmware drew, not what the panel looks like: no backlight, "+
			"no viewing angle, no refresh artefacts.")))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}
