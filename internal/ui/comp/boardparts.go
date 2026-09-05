// The parts of a board that are not its screen: its lamps, the things somebody
// can press, and the ball they can roll.
//
// Here rather than in a panel because two windows draw them now - the node
// window's Hardware tab, where a board is recognised and poked, and the
// bring-up window, where it is checked against its own profile. A second copy
// of this would be a second set of answers to "which pin is up", and the pin a
// direction is on took working out from two disagreeing frames.
//
// One renderer per kind rather than one per board. What varies between boards
// is which of these appear and on which pins, and that is declared in the
// board's own file - so a new board adds no drawing code at all, and a part
// that draws wrongly draws wrongly everywhere at once, where it will be seen.
package comp

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// BoardControls draws a board's parts and reports what was pressed.
//
// It owns the widgets, because widget identity is address: a control rebuilt
// each frame loses the press half way through, and a press half-registered on
// a board is a button that reads as broken.
type BoardControls struct {
	buttons map[int]*widget.Clickable
	down    map[int]bool
}

// Press is one pin changing state under somebody's finger.
type Press struct {
	Pin  int
	Down bool
}

// Presses is every pin whose button has changed since the last frame.
//
// Edges rather than levels, because that is what the firmware counts. A
// trackball's directions are pins like any other: what makes one a step rather
// than a hold is the firmware counting changes of level, so pressing and
// letting go rolls the ball two notches, which is what rolling it past a line
// does.
func (c *BoardControls) Presses(panel *hw.Panel) []Press {
	if panel == nil {
		return nil
	}
	var pins []int
	for _, part := range panel.PartsOfKind(hw.Button) {
		if part.Pin != hw.PinNone {
			pins = append(pins, part.Pin)
		}
	}
	for _, part := range panel.PartsOfKind(hw.Ball) {
		for _, pin := range part.Pins {
			if pin != hw.PinNone {
				pins = append(pins, pin)
			}
		}
	}
	var out []Press
	for _, pin := range pins {
		btn := c.For(pin)
		down := btn.Pressed()
		if down == c.down[pin] {
			continue
		}
		if c.down == nil {
			c.down = map[int]bool{}
		}
		c.down[pin] = down
		out = append(out, Press{Pin: pin, Down: down})
	}
	return out
}

// For is this pin's control, kept across frames.
func (c *BoardControls) For(pin int) *widget.Clickable {
	if c.buttons == nil {
		c.buttons = map[int]*widget.Clickable{}
	}
	if b, ok := c.buttons[pin]; ok {
		return b
	}
	b := &widget.Clickable{}
	c.buttons[pin] = b
	return b
}

// Lamps draws the board's lights, and says where a board has none.
func (c *BoardControls) Lamps(t *theme.Theme, gtx layout.Context,
	panel *hw.Panel) layout.Dimensions {

	parts := panel.PartsOfKind(hw.Lamp)
	if len(parts) == 0 {
		return Text(t, t.Sz.Caption, t.P.Faint, "no lamp declared")(gtx)
	}
	children := make([]layout.FlexChild, 0, len(parts)*2)
	for i := range parts {
		part := parts[i]
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return lamp(t, gtx, part)
		}))
		children = append(children, layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

// lamp is one light. Whether it is lit is not modelled: the pin is declared and
// nothing watches it. Drawn as an outline rather than as an unlit lamp, because
// "off" and "not modelled" are different facts and the second is about us.
func lamp(t *theme.Theme, gtx layout.Context, part hw.Part) layout.Dimensions {
	d := gtx.Dp(unit.Dp(10))
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(d, d)
			rr := d / 2
			paint.FillShape(gtx.Ops, t.P.Rule, clip.RRect{
				Rect: image.Rectangle{Max: size}, NE: rr, NW: rr, SE: rr, SW: rr,
			}.Op(gtx.Ops))
			inner := image.Rect(1, 1, d-1, d-1)
			ir := (d - 2) / 2
			paint.FillShape(gtx.Ops, t.P.Panel, clip.RRect{
				Rect: inner, NE: ir, NW: ir, SE: ir, SW: ir,
			}.Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		}),
		layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout),
		layout.Rigid(Text(t, t.Sz.Caption, t.P.Dim, part.Name)),
	)
}

// Buttons draws what somebody can press, and says where a board has none.
func (c *BoardControls) Buttons(t *theme.Theme, gtx layout.Context,
	panel *hw.Panel) layout.Dimensions {

	parts := panel.PartsOfKind(hw.Button)
	if len(parts) == 0 {
		return Text(t, t.Sz.Caption, t.P.Faint, "no button declared")(gtx)
	}
	children := make([]layout.FlexChild, 0, len(parts)*2)
	for i := range parts {
		part := parts[i]
		if part.Pin == hw.PinNone {
			// The board says it has none. Said rather than omitted: an absence
			// somebody recorded is worth more than a gap that reads as nobody
			// having looked. Not a control, because there is nothing to press.
			children = append(children, layout.Rigid(
				Pill(t, t.P.Faint, part.Name+" - none on this board")))
			children = append(children, layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout))
			continue
		}
		btn := c.For(part.Pin)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return btn.Layout(gtx, Pill(t, pressInk(t, btn), part.Name))
		}))
		children = append(children, layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

// Ball draws a trackball as the four ways it can be rolled.
//
// Arrows rather than a drawn ball, because what reaches the firmware is not a
// position: each direction is a plain line that changes level as the ball turns
// past it, and the firmware counts the changes. One press is one step, which is
// exactly what these produce.
func (c *BoardControls) Ball(t *theme.Theme, gtx layout.Context,
	panel *hw.Panel) layout.Dimensions {

	parts := panel.PartsOfKind(hw.Ball)
	if len(parts) == 0 {
		return layout.Dimensions{}
	}
	part := parts[0]
	// Named in the order a board declares them, and only as many as it has: a
	// trackball with fewer lines than four is a declaration to fix rather than
	// a thing to draw four arrows for.
	dirs := [4]struct {
		way   Arrow
		label string
	}{
		{ArrowUp, "up"}, {ArrowDown, "down"},
		{ArrowLeft, "left"}, {ArrowRight, "right"},
	}
	pins := part.Pins
	if len(pins) > len(dirs) {
		pins = pins[:len(dirs)]
	}
	children := make([]layout.FlexChild, 0, len(pins)*2+1)
	children = append(children, layout.Rigid(
		Text(t, t.Sz.Caption, t.P.Dim, part.Name)))
	children = append(children, layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout))
	for i, pin := range pins {
		if pin == hw.PinNone {
			continue
		}
		dir := dirs[i]
		btn := c.For(pin)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return btn.Layout(gtx, ArrowPill(t, pressInk(t, btn), dir.way, dir.label))
		}))
		children = append(children, layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

// pressInk is the accent while a control is held, so a press is visible on the
// board as well as on the pin.
func pressInk(t *theme.Theme, btn *widget.Clickable) color.NRGBA {
	if btn.Pressed() {
		return t.P.Accent
	}
	return t.P.Dim
}
