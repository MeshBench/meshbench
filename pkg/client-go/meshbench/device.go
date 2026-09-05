package meshbench

import (
	"context"
	"fmt"
	"time"
)

// Device is a running board a script can look at and prod: read what its display
// is showing, capture it as an image, press its buttons, type at its keyboard,
// and touch its panel. All of it works headless - the display is the
// framebuffer the controller holds, not a picture of anybody's desktop - which
// is the point: a board test that needs a screen in front of it does not run
// in CI.
type Device struct{ n Node }

// Device is this node as a device to drive: its screen, buttons and panel.
func (n Node) Device() Device { return Device{n} }

// Screen is what a board's display is showing, as numbers rather than a
// picture. Enough to answer "did anything change" after a press or a touch;
// for the picture itself, use Screenshot.
type Screen struct {
	// HasScreen is false when the board has drawn nothing yet, or has no
	// display at all - the other fields are meaningless then.
	HasScreen bool `json:"has_screen"`
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	BPP       int  `json:"bpp"`
	On        bool `json:"on"`
	// Lit is how many framebuffer bytes are non-zero - how much is lit.
	Lit int `json:"lit"`
	// Digest identifies the frame: two screens with the same Digest are the
	// same picture, which Lit cannot promise. It is what WaitScreen watches.
	Digest string `json:"digest"`
}

// Screen reads what this board's display is currently showing.
func (b Device) Screen(ctx context.Context) (Screen, error) {
	var s Screen
	return s, b.n.w.CallInto(ctx, "board.screen", map[string]any{"node": b.n.name}, &s)
}

// Shot is a captured display: a PNG written under the node's own work
// directory, with the frame's dimensions.
type Shot struct {
	Path   string `json:"path"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	BPP    int    `json:"bpp"`
	On     bool   `json:"on"`
}

// Screenshot writes the board's display to a PNG and returns where it landed.
// The frame is exactly what the controller holds, at the size it holds it.
func (b Device) Screenshot(ctx context.Context) (Shot, error) {
	var s Shot
	return s, b.n.w.CallInto(ctx, "board.screenshot", map[string]any{"node": b.n.name}, &s)
}

// Reset restarts the board, the way pressing its own reset button does.
//
// Torn down and built again from the same flash: whatever the firmware wrote
// survives and whatever it held in memory does not. It answers when the board
// is back up.
func (b Device) Reset(ctx context.Context) error {
	_, err := b.n.w.Call(ctx, "board.reset", map[string]any{"node": b.n.name})
	return err
}

// Press holds a button pin down, or releases it. Held rather than clicked
// because the firmware cares: MeshCore wakes a sleeping display on a press and
// powers the board off on a long one, so a caller times the release itself.
func (b Device) Press(ctx context.Context, pin int, down bool) error {
	return b.n.w.Do(ctx, "board.press",
		map[string]any{"node": b.n.name, "pin": pin, "down": down})
}

// Tap presses a button and lets go - the ordinary click, for when the hold
// does not matter.
func (b Device) Tap(ctx context.Context, pin int) error {
	if err := b.Press(ctx, pin, true); err != nil {
		return err
	}
	return b.Press(ctx, pin, false)
}

// Type enters text at the board's own keyboard, one character at a time -
// which is what the keyboard sends, and what the firmware polls for.
func (b Device) Type(ctx context.Context, text string) error {
	return b.n.w.Do(ctx, "board.key", map[string]any{"node": b.n.name, "text": text})
}

// Touch puts a finger on the panel at a point (down true) or lifts it off
// (down false).
func (b Device) Touch(ctx context.Context, x, y int, down bool) error {
	return b.n.w.Do(ctx, "board.touch",
		map[string]any{"node": b.n.name, "x": x, "y": y, "down": down})
}

// TapAt touches a point and lifts off - a tap on the panel.
func (b Device) TapAt(ctx context.Context, x, y int) error {
	if err := b.Touch(ctx, x, y, true); err != nil {
		return err
	}
	return b.Touch(ctx, x, y, false)
}

// WaitScreen waits until the display changes from what it shows now and returns
// the new frame, or fails with what it was still showing when the timeout ran
// out. This is the honest way to check an input: half duplex eats stimuli - a
// board handed a packet while transmitting never hears it - so a tap followed
// by an immediate screen read will intermittently read the frame from before
// the tap landed. Change is by Digest, so a redraw that keeps the same number
// of lit pixels still counts.
func (b Device) WaitScreen(ctx context.Context, timeout time.Duration) (Screen, error) {
	before, err := b.Screen(ctx)
	if err != nil {
		return Screen{}, err
	}
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return before, fmt.Errorf("board %s: the screen did not change within %s",
				b.n.name, timeout)
		}
		select {
		case <-ctx.Done():
			return before, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
		now, err := b.Screen(ctx)
		if err != nil {
			return before, err
		}
		if now.Digest != before.Digest {
			return now, nil
		}
	}
}

// Radio is what this node's radio is set to - the same thing the workbench
// shows under Radio: the frequency, spreading factor, bandwidth and the rest
// the model assumes, and, for a node that is running, what it reports back and
// where the two differ. The shape is left open because a repeater and a
// companion answer it differently, and both are worth having whole.
func (n Node) Radio(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	return out, n.w.CallInto(ctx, "node.radio", map[string]any{"node": n.name}, &out)
}
