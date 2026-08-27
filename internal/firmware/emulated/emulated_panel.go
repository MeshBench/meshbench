package emulated

import "github.com/MeshBench/meshbench/internal/firmware/emulated/peripheral"

// What somebody can see on an emulated board, and what they can do to it.
//
// Kept beside the node rather than inside it because they are one shape of
// thing: every one of these is the far end of a socket the machine was given
// at bring-up, and none of them exists on a board whose profile does not
// declare the part.

// Screen is the last picture this board's display sent, and whether there is
// one at all.
//
// The last return says which: a board that declares no display and a board
// whose display has drawn nothing are different facts, and drawing an empty
// picture for both would report the first as the second.
func (e *EmulatedNode) Screen() (width, height, bpp int, on bool, bits []byte, have bool) {
	if e.Panel == nil {
		return 0, 0, 0, false, nil, false
	}
	f, _ := e.Panel.Frame()
	if f == nil {
		return 0, 0, 0, false, nil, false
	}
	return f.Width, f.Height, f.BPP, f.On, f.Bits, true
}

// PressButton holds one of this board's buttons down or lets it go.
func (e *EmulatedNode) PressButton(pin int, down bool) error {
	if e.Buttons == nil {
		return peripheral.ErrNoButtons()
	}
	return e.Buttons.Press(pin, down)
}

// ButtonHeld reports whether a pin is being held.
func (e *EmulatedNode) ButtonHeld(pin int) bool {
	return e.Buttons != nil && e.Buttons.Held(pin)
}

// TypeKey types one character at this board's keyboard.
func (e *EmulatedNode) TypeKey(ch byte) error {
	if e.Buttons == nil {
		return peripheral.ErrNoButtons()
	}
	return e.Buttons.Key(ch)
}

// TouchScreen reports where this board's panel is being touched.
func (e *EmulatedNode) TouchScreen(x, y int, down bool) error {
	if e.Buttons == nil {
		return peripheral.ErrNoButtons()
	}
	return e.Buttons.Touch(x, y, down)
}

// SetMeter is what the board's converter reads on a channel from now on.
//
// The number rather than a voltage, because the scale between them is the
// board's divider and belongs with the board. Sent down the same channel the
// buttons use: the cell is an input to the firmware like any other, and one
// channel per board is what keeps it that way.
func (e *EmulatedNode) SetMeter(channel int, raw uint16) error {
	if e.Buttons == nil {
		return peripheral.ErrNoButtons()
	}
	e.BatRaw = raw
	return e.Buttons.Analog(channel, raw)
}
