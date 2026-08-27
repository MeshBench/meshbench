// What a board physically presents to a person: its screen, its lamps, the
// buttons somebody can press.
//
// Declared rather than drawn. There are two dozen boards here and there will
// be more, and a panel written by hand for each is where tedium turns into
// error - one board's lamp quietly reading the wrong pin looks exactly like
// firmware that never lit it. So a board says what it has, in the same file
// and the same style as it says where its radio sits, and one renderer draws
// whatever it said.
//
// This is not a description of an interface. A lamp's pin is a fact about the
// board, and the emulator needs it too - to know which output to watch - so
// the machine's wiring reads the same declaration. Two lists that must agree
// eventually do not.
package board

import "fmt"

// PinNone marks a part the board does not carry, distinctly from one whose
// pin nobody has established. A board that says "no button" is telling us
// something; a board that says nothing is not.
const PinNone = -1

// PartKind is what a part of a board is, which decides how it is drawn and
// how it reaches the guest.
type PartKind int

const (
	// Lamp is an LED the firmware drives. Watched, never driven: it is an
	// output, and the whole point of showing it is to see what the firmware
	// did with it.
	Lamp PartKind = iota
	// Button is a momentary switch a person can hold down. Driven, never
	// watched.
	Button
	// Keys is a keyboard that answers over a bus rather than a pin per key -
	// on the boards here, a second microcontroller with its own firmware.
	Keys
	// Touch is a touch panel, which reports a point rather than a state.
	Touch
	// Ball is a trackball: four direction lines that pulse as it turns, and
	// a click declared separately as a Button, because that is how the
	// firmware reads them.
	Ball
	// Card is a card slot, on these boards a third device on the bus the
	// radio and the display already share, told apart by its own select.
	//
	// Backed by a file per node, which is what makes a node's storage
	// something an operator can look inside afterwards - a real handheld only
	// offers that by taking the card out.
	Card
	// GPS is a receiver on the board's second serial port, which is where
	// every variant here opens one. It reports rather than being driven: what
	// it says is where the scenario put the node, so moving a node on the map
	// moves it on the handheld.
	GPS
	// Meter is something the board measures rather than shows - on these
	// boards, the divider its cell is read through. It is here for the same
	// reason a lamp is: the pin is a fact about the board, and the machine
	// needs it as much as the interface does.
	//
	// An unmodelled one is not quiet. A firmware that starts a conversion and
	// waits for a converter nobody built waits for ever, which is what the
	// published companion build did here.
	Meter
)

func (k PartKind) String() string {
	switch k {
	case Button:
		return "button"
	case Keys:
		return "keyboard"
	case Touch:
		return "touch"
	case Ball:
		return "trackball"
	case Meter:
		return "meter"
	case GPS:
		return "GPS"
	case Card:
		return "card slot"
	}
	return "lamp"
}

// Bus is how a part is reached.
type Bus int

const (
	// BusPin is an ordinary GPIO, which most parts are.
	BusPin Bus = iota
	BusI2C
	BusSPI
)

func (b Bus) String() string {
	switch b {
	case BusI2C:
		return "I2C"
	case BusSPI:
		return "SPI"
	}
	return "GPIO"
}

// Ink is what a screen can show, which is not the same question as how big it
// is. A monochrome panel that draws four greys is drawing a lie.
type Ink int

const (
	Mono Ink = iota
	RGB565
	EPaper
)

func (i Ink) String() string {
	switch i {
	case RGB565:
		return "colour"
	case EPaper:
		return "e-paper"
	}
	return "mono"
}

// Screen is the display a board carries, or nil where it carries none.
//
// Controller matters as much as size: the same 128x64 panel is an SSD1306 on
// one board and an SH1106 on another, and they differ by a column offset that
// shifts the whole picture sideways if it is got wrong.
type Screen struct {
	Controller string
	Bus        Bus

	// Addr is the I2C address, where the bus is I2C.
	Addr uint8
	// CS and DC are the SPI chip select and the command/data line. On these
	// boards the select is an ordinary GPIO the firmware drives itself, not
	// the controller's own, because the display shares its bus with the radio.
	CS, DC int

	WidthPx, HeightPx int
	Ink               Ink
}

// Part is one of the things on a board that is not its screen.
type Part struct {
	Kind PartKind
	// Name is what is written beside it, in the board's own words where the
	// board has any - a Heltec's button is a PRG button and calling it
	// anything else helps nobody.
	Name string

	Bus  Bus
	Pin  int
	Addr uint8
	// Pins is the set a part needs where one is not enough: a trackball's
	// four directions, in the order up, down, left, right.
	Pins []int

	// FullScaleMV is what a Meter reads at the top of its range, in the units
	// the firmware reports - so for a battery divider, the cell voltage that
	// would put the converter at full scale.
	//
	// It is the divider and the converter's range together, because that is
	// the only form in which it can be checked: the number is right when the
	// firmware's own arithmetic, applied to what we injected, prints what the
	// simulation says the cell is at.
	FullScaleMV int

	// Rotate is how the panel is mounted against the picture drawn on it, in
	// degrees clockwise, for a touch layer whose axes are not the screen's.
	//
	// A T-Deck's panel is fitted turned a quarter: its firmware reads a point
	// and computes screen x from the panel's y, and screen y backwards from
	// the panel's x. So a tap has to be turned the other way before it is
	// sent, or every one lands somewhere else - which looks exactly like a
	// touch layer that is not wired at all.
	Rotate int

	// ActiveLow says the firmware reads this part pressed when its pin is
	// low, which is what a button with a pull-up looks like. Recorded because
	// getting it backwards produces a board that is either always pressed or never
	// pressed, and both look like the part is not wired.
	ActiveLow bool
}

// Panel is everything a board shows and everything somebody can press on it.
//
// Order is drawing order. Lamps go above the screen and everything else below,
// which follows from the kind rather than from a position on the part: none of
// these is a photograph, and a schematic that reads correctly is worth more
// than a likeness maintained two dozen times.
type Panel struct {
	Screen *Screen
	Parts  []Part
}

// RawPoint turns a point on the drawn picture into the point the panel itself
// would report, undoing however it is mounted.
//
// Here rather than in the interface because the mounting is a fact about the
// board, and the interface should not have to know which way round anybody
// screwed a panel in.
func (p Part) RawPoint(x, y, width, height int) (int, int) {
	switch ((p.Rotate % 360) + 360) % 360 {
	case 90:
		return height - 1 - y, x
	case 180:
		return width - 1 - x, height - 1 - y
	case 270:
		return y, width - 1 - x
	}
	return x, y
}

// PartsOfKind is the parts of one kind, in declared order.
func (p *Panel) PartsOfKind(k PartKind) []Part {
	if p == nil {
		return nil
	}
	var out []Part
	for _, part := range p.Parts {
		if part.Kind == k {
			out = append(out, part)
		}
	}
	return out
}

// HasAnything reports whether there is something worth drawing. A board with
// no screen and no parts still gets a tab, but the tab says so rather than
// showing an empty frame.
func (p *Panel) HasAnything() bool {
	return p != nil && (p.Screen != nil || len(p.Parts) > 0)
}

// Validate reports what is wrong with a panel, if anything.
//
// There is no per-board code to review here, so this is the review: a
// declaration that cannot be drawn, or that quietly claims a pin the radio is
// already using, is caught against the board that introduced it rather than
// on the day somebody opens that node.
func (p *Panel) Validate(taken map[int]string) error {
	if p == nil {
		return nil
	}
	if s := p.Screen; s != nil {
		if s.Controller == "" {
			return fmt.Errorf("screen has no controller named")
		}
		if s.WidthPx <= 0 || s.HeightPx <= 0 {
			return fmt.Errorf("%s screen has no size", s.Controller)
		}
		switch s.Bus {
		case BusI2C:
			if s.Addr == 0 {
				return fmt.Errorf("%s is on I2C with no address", s.Controller)
			}
		case BusSPI:
			if s.CS == 0 || s.DC == 0 {
				return fmt.Errorf("%s is on SPI with no select or command pin", s.Controller)
			}
		default:
			return fmt.Errorf("%s is on no bus", s.Controller)
		}
	}
	seen := map[int]string{}
	for _, part := range p.Parts {
		if part.Name == "" {
			return fmt.Errorf("a %s has no name", part.Kind)
		}
		if part.Bus == BusI2C && part.Addr == 0 {
			return fmt.Errorf("%s is on I2C with no address", part.Name)
		}
		if part.Kind == Meter && part.FullScaleMV <= 0 {
			return fmt.Errorf("%s measures something with no scale to read it against", part.Name)
		}
		if part.Kind == Ball && len(part.Pins) != 4 {
			return fmt.Errorf("%s needs four direction lines, has %d", part.Name, len(part.Pins))
		}
		pins := part.Pins
		if part.Bus == BusPin && part.Pin != PinNone && len(pins) == 0 {
			pins = []int{part.Pin}
		}
		for _, pin := range pins {
			if pin == PinNone {
				continue
			}
			if who, dup := seen[pin]; dup {
				return fmt.Errorf("%s and %s are both on pin %d", who, part.Name, pin)
			}
			if who, dup := taken[pin]; dup {
				return fmt.Errorf("%s is on pin %d, which is the radio's %s", part.Name, pin, who)
			}
			seen[pin] = part.Name
		}
	}
	return nil
}

// radioPins is what the radio has already claimed on a board, so a part
// cannot quietly take one of them.
//
// A collision here is the worst kind of mistake to debug: the board still
// boots, the radio still reports a chip, and then something intermittently
// stops working because two things are driving one line.
func (b Board) radioPins() map[int]string {
	taken := map[int]string{}
	if w := b.QEMU; w != nil {
		for pin, who := range map[int]string{w.NSS: "chip select", w.Busy: "busy", w.DIO1: "interrupt"} {
			if pin != 0 {
				taken[pin] = who
			}
		}
		if w.FEM != 0 {
			taken[w.FEM] = "front-end module"
		}
	}
	return taken
}
