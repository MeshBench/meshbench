package workbench

import (
	"fmt"
	"image"
	"strings"

	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
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
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start,
					Spacing: layout.SpaceEnd}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return p.device(t, gtx, panel, st)
					}),
					layout.Rigid(layout.Spacer{Width: t.Sp.L}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return p.hardwareFacts(t, gtx, panel, st)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return p.card(t, gtx, s)
							}),
						)
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.lastWords(t, gtx, s)
			}),
		)
	})(gtx)
}

// lastWords is the tail of what the board printed, under the picture of it.
//
// Here because this is where somebody is standing when a board draws nothing:
// looking at a drawn panel that is blank, deciding whether the emulator is
// broken or the firmware never started. The whole of it is one tab away and
// the strip says so; four lines is enough to tell a board that is booting from
// one that has said nothing at all.
func (p *nodeWindowPanel) lastWords(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	p.askSerial()
	lines := outputSummary(p.node, s, 4)
	if len(lines) == 0 {
		// Silence is a finding, so it is drawn rather than skipped - a strip
		// that disappears when there is nothing looks like a strip that has
		// not loaded.
		lines = []string{"nothing on this board's serial port yet"}
	}
	kids := []layout.FlexChild{
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, "last words")),
		layout.Rigid(layout.Spacer{Height: t.Sp.XXS}.Layout),
	}
	for i := range lines {
		line := trimLine(lines[i], 110)
		kids = append(kids, layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Dim, line)))
	}
	kids = append(kids,
		layout.Rigid(layout.Spacer{Height: t.Sp.XXS}.Layout),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
			"the whole of it, and the emulator's own, are in the Output tab")),
	)
	return layout.Inset{Top: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
	})
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
			layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.ball(t, gtx, panel)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.typingNote(t, gtx, panel)
			}),
		)
	})(gtx)
}

// typingNote says how to type at a board that has a keyboard.
//
// Said rather than left to be discovered. Keys go to whatever has focus, and
// a keyboard that silently needs a click first is one somebody decides is
// broken - which is exactly what happened.
func (p *nodeWindowPanel) typingNote(t *theme.Theme, gtx layout.Context,
	panel *scenario.Panel) layout.Dimensions {

	if !p.typeable(panel) {
		return layout.Dimensions{}
	}
	note := "click the screen, then type - keys go to the board's own keyboard"
	if gtx.Focused(&p.screenKeys) {
		note = "typing goes to the board - click elsewhere to stop"
	}
	return layout.Inset{Top: t.Sp.XS}.Layout(gtx,
		comp.Text(t, t.Sz.Caption, t.P.Faint, note))
}

// typeable reports whether this board has a keyboard to type at.
func (p *nodeWindowPanel) typeable(panel *scenario.Panel) bool {
	return panel != nil && len(panel.PartsOfKind(scenario.Keys)) > 0
}

// boardKeys turns typing into keys at the board's own keyboard.
//
// Focus is taken by clicking the drawn panel, which is what somebody does
// with a handheld before typing on it - and what stops every keystroke
// meant for the workbench being swallowed by whichever node window happens
// to be open.
//
// Characters rather than scan codes, because that is what this keyboard
// sends: on these boards it is a second microcontroller that has already
// decided what was pressed. The two keys that are not characters and that a
// text field cannot do without are sent as the bytes it expects for them.
func (p *nodeWindowPanel) boardKeys(gtx layout.Context, s *state.Snapshot) {
	panel := p.boardPanel(s)
	if !p.typeable(panel) || p.OnDo == nil {
		return
	}
	for {
		ev, ok := gtx.Event(
			key.FocusFilter{Target: &p.screenKeys},
			key.Filter{Focus: &p.screenKeys},
		)
		if !ok {
			break
		}
		switch e := ev.(type) {
		case key.EditEvent:
			if e.Text != "" {
				p.OnDo("board.key", map[string]any{"node": p.node, "text": e.Text})
			}
		case key.Event:
			if e.State != key.Press {
				continue
			}
			// Backspace and return, as their own bytes. A field somebody is
			// typing a name into is unusable without them.
			switch e.Name {
			case key.NameDeleteBackward:
				p.OnDo("board.key", map[string]any{"node": p.node, "text": "\b"})
			case key.NameReturn, key.NameEnter:
				p.OnDo("board.key", map[string]any{"node": p.node, "text": "\r"})
			}
		}
	}
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
	touch := panel.PartsOfKind(scenario.Touch)
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
		// Clicking the board is also what puts the keyboard on it, the way
		// picking a handheld up is what precedes typing on it.
		if pe.Kind == pointer.Press && p.typeable(panel) {
			gtx.Execute(key.FocusCmd{Tag: &p.screenKeys})
		}
		// Turned back into the panel's own axes before it leaves. What was
		// clicked is a point on the picture; what the firmware reads is a
		// point on a panel that may be screwed in sideways, and on the one
		// board here that has a touch layer it is.
		rx, ry := touch[0].RawPoint(x, y, sc.WidthPx, sc.HeightPx)
		p.OnDo("board.touch", map[string]any{
			"node": p.node, "x": rx, "y": ry,
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
	// A trackball's directions are pins like any other. What makes one a step
	// rather than a hold is the firmware, which counts changes of level - so
	// pressing and letting go rolls the ball two notches, which is what
	// rolling it past a line does.
	pins := make([]int, 0, len(panel.Parts))
	for _, part := range panel.PartsOfKind(scenario.Button) {
		if part.Pin != scenario.PinNone {
			pins = append(pins, part.Pin)
		}
	}
	for _, part := range panel.PartsOfKind(scenario.Ball) {
		for _, pin := range part.Pins {
			if pin != scenario.PinNone {
				pins = append(pins, pin)
			}
		}
	}
	for _, pin := range pins {
		btn := p.buttonFor(pin)
		down := btn.Pressed()
		if down == p.buttonDown[pin] {
			continue
		}
		if p.buttonDown == nil {
			p.buttonDown = map[int]bool{}
		}
		p.buttonDown[pin] = down
		p.OnDo("board.press", map[string]any{
			"node": p.node, "pin": pin, "down": down})
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
		rows = append(rows, row{partLabel(part), partFact(part)})
	}
	// The radio the board has and the simulation has not.
	//
	// Every ESP32 part carries Wi-Fi and Bluetooth, and neither is modelled
	// here: this simulator has one radio and it is the LoRa transceiver. Said
	// on the tab because the alternative is a firmware that appears to join a
	// network and an operator with no way to know it never could. Derived
	// from the MCU rather than declared per board, so it cannot drift.
	if st != nil {
		if b, err := scenario.BoardByName(st.Board); err == nil &&
			strings.HasPrefix(strings.ToUpper(b.MCU), "ESP32") {
			rows = append(rows, row{"wireless",
				"Wi-Fi and Bluetooth - stubbed, never on the air"})
		}
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

// partLabel is what a part is called on the left of the facts column.
//
// The kind and the name, unless the board named it after its kind - a T-Deck's
// keyboard is called "keyboard", and "keyboard keyboard" reads as a bug in the
// interface rather than as a board that had nothing more to add.
func partLabel(part scenario.Part) string {
	kind := part.Kind.String()
	if part.Name == "" || strings.EqualFold(part.Name, kind) {
		return kind
	}
	return kind + " " + part.Name
}

// partFact is the one thing worth reading off a part: where it is.
//
// Where rather than what, because what it is was said on the left. A pin
// number, a bus address or a port is the fact somebody comes to this column
// for - it is what they will compare against the board's own documentation
// when something does not work.
func partFact(part scenario.Part) string {
	if part.Bus == scenario.BusI2C {
		return fmt.Sprintf("I2C 0x%02X", part.Addr)
	}
	switch part.Kind {
	case scenario.Ball:
		return fmt.Sprintf("pins %v, up down left right", part.Pins)
	case scenario.Meter:
		// The scale as well as the pin, because the scale is what the number
		// on the board's own screen has to be checked against.
		return fmt.Sprintf("pin %d, full scale %.1f V",
			part.Pin, float64(part.FullScaleMV)/1000)
	case scenario.GPS:
		return "second serial port"
	}
	if part.Pin == scenario.PinNone {
		return "none on this board"
	}
	return fmt.Sprintf("pin %d", part.Pin)
}
