// Reaching a board's own controls: its buttons, its keyboard, its touch panel,
// and what its display is showing as a result.
//
// The pair is what makes any of them worth having. A control that reaches the
// hardware silently is indistinguishable from one that does not reach it at
// all, so every one of these has board.screen beside it: press, then ask
// whether anything on the panel changed.
package session

import (
	"fmt"
	"strconv"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerBoardInput(st *state.Store, s *Sim) {
	// Held rather than clicked, because the firmware behind these pins cares:
	// MeshCore wakes a sleeping display on a press and powers the board off on
	// a long one, and a verb that could only produce a tap could reach neither.
	st.HandleSpec("board.press", state.Spec{
		What: "hold one of a board's own buttons down, or let it go, so a long " +
			"press reaches the firmware as a long press rather than as a tap",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true,
				What: "which board; refused when absent, when the node is not " +
					"running, or when its backend has no buttons"},
			{Name: "pin", Type: state.ParamNumber, Required: true,
				What: "the GPIO the button sits on, as the board profile " +
					"declares it; refused when absent or not a number"},
			{Name: "down", Type: state.ParamBool,
				What: "true holds it, false releases it; absent counts as a release"},
		},
		Returns: []string{"node", "pin", "down"},
		Answers: "The answer repeats what was asked, which is the acknowledgement " +
			"that the press reached the board at all. Whether the firmware did " +
			"anything with it is board.screen's question.",
		Example: &state.Example{
			Params: map[string]any{"node": "West Lomond", "pin": 0, "down": true},
			What:   "hold the PRG button down",
		},
	}, func(w *state.World, p any) (any, error) {
		m, _ := p.(map[string]any)
		name, _ := m["node"].(string)
		pinF, okPin := numField(p, "pin")
		pin := int(pinF)
		down, _ := m["down"].(bool)
		if name == "" || !okPin {
			return nil, fmt.Errorf("board.press needs a node and a pin")
		}
		n, found := s.liveEngine().NodeByName(name)
		if !found || n.Firmware == nil {
			return nil, fmt.Errorf("%s is not running", name)
		}
		presser, ok := n.Firmware.Backend.(interface{ PressButton(int, bool) error })
		if !ok {
			return nil, fmt.Errorf("%s is not a board with buttons", name)
		}
		if err := presser.PressButton(pin, down); err != nil {
			return nil, err
		}
		what := "released"
		if down {
			what = "held"
		}
		w.Say(fmt.Sprintf("%s: %s pin %d", name, what, pin))
		return map[string]any{"node": name, "pin": pin, "down": down}, nil
	})

	st.HandleSpec("board.key", state.Spec{
		What: "type at the board's own keyboard, a character at a time, which is " +
			"what the hardware sends: it holds the last key pressed and the " +
			"firmware polls it",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true,
				What: "which board; refused when absent, when the node is not " +
					"running, or when its backend has no keyboard"},
			{Name: "text", Type: state.ParamString, Required: true,
				What: "the characters to send, each one a keypress; refused when " +
					"absent or empty"},
		},
		Returns: []string{"node", "typed"},
		Answers: "`typed` is how many characters were sent, not how many the " +
			"firmware read: it polls, so typing faster than it polls loses keys.",
		Example: &state.Example{
			Params: map[string]any{"node": "West Lomond", "text": "hello"},
			What:   "type a word at the board's keyboard",
		},
	}, func(w *state.World, p any) (any, error) {
		m, _ := p.(map[string]any)
		name, _ := m["node"].(string)
		text, _ := m["text"].(string)
		if name == "" || text == "" {
			return nil, fmt.Errorf("board.key needs a node and some text")
		}
		n, found := s.liveEngine().NodeByName(name)
		if !found || n.Firmware == nil {
			return nil, fmt.Errorf("%s is not running", name)
		}
		typer, ok := n.Firmware.Backend.(interface{ TypeKey(byte) error })
		if !ok {
			return nil, fmt.Errorf("%s is not a board with a keyboard", name)
		}
		// One character at a time, because that is what the keyboard sends:
		// it answers with the last key pressed and the firmware polls it.
		for i := 0; i < len(text); i++ {
			if err := typer.TypeKey(text[i]); err != nil {
				return nil, err
			}
		}
		return map[string]any{"node": name, "typed": len(text)}, nil
	})

	st.HandleSpec("board.touch", state.Spec{
		What: "put a finger on the board's panel at a point, or take it off, in " +
			"the panel's own pixels rather than the drawn screen's",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true,
				What: "which board; refused when absent, when the node is not " +
					"running, or when its backend has no touch panel"},
			{Name: "x", Type: state.ParamNumber, Required: true,
				What: "the column, from the panel's own left edge; refused when " +
					"absent or not a number"},
			{Name: "y", Type: state.ParamNumber, Required: true,
				What: "the row, from the panel's own top edge; refused when " +
					"absent or not a number"},
			{Name: "down", Type: state.ParamBool,
				What: "true touches, false lifts off; absent counts as a lift off"},
		},
		Returns: []string{"node", "x", "y", "down"},
		Example: &state.Example{
			Params: map[string]any{
				"node": "West Lomond", "x": 120, "y": 80, "down": true,
			},
			What: "touch the middle of the panel",
		},
	}, func(w *state.World, p any) (any, error) {
		m, _ := p.(map[string]any)
		name, _ := m["node"].(string)
		xf, okX := numField(p, "x")
		yf, okY := numField(p, "y")
		down, _ := m["down"].(bool)
		if name == "" || !okX || !okY {
			return nil, fmt.Errorf("board.touch needs a node and a point")
		}
		n, found := s.liveEngine().NodeByName(name)
		if !found || n.Firmware == nil {
			return nil, fmt.Errorf("%s is not running", name)
		}
		toucher, ok := n.Firmware.Backend.(interface{ TouchScreen(int, int, bool) error })
		if !ok {
			return nil, fmt.Errorf("%s is not a board with a touch panel", name)
		}
		if err := toucher.TouchScreen(int(xf), int(yf), down); err != nil {
			return nil, err
		}
		// Said, because a control that reaches the board silently is
		// indistinguishable from one that does not reach it at all - which is
		// exactly the question somebody asks when tapping a drawn screen does
		// nothing. Presses say the same thing.
		what := "lifted off"
		if down {
			what = "touched"
		}
		w.Say(fmt.Sprintf("%s: %s at %d,%d on its panel", name, what, int(xf), int(yf)))
		return map[string]any{"node": name, "x": int(xf), "y": int(yf), "down": down}, nil
	})

	// Not a picture. Enough to answer "did anything change" from a script or
	// a control socket, which is the question every check of a touch or a
	// keypress comes down to - and answering it by taking a screenshot of
	// somebody's desktop is not an answer.
	st.HandleSpec("board.screen", state.Spec{
		What: "measure what a board's own display is showing as numbers rather " +
			"than as a picture, so a script can tell whether a press or a " +
			"keystroke changed anything",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true, Primary: true,
				What: "which board; refused when absent, when the node is not " +
					"running, or when its backend has no display"},
		},
		Returns: []string{"node", "has_screen", "width", "height", "bpp", "on",
			"lit", "digest"},
		Answers: "A board whose backend models a display but has drawn nothing " +
			"yet answers with `has_screen` false and nothing else, which is a " +
			"fact about the board rather than a failure. Otherwise `lit` counts " +
			"the non-zero bytes of the framebuffer, for how much is on, and " +
			"`digest` is a hex hash of the whole frame, for whether it is the " +
			"same frame - which is what a wait for the screen to change compares.",
		Example: &state.Example{
			Params: "West Lomond", What: "check whether the display has changed",
		},
	}, func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "node")
		if name == "" {
			return nil, fmt.Errorf("board.screen needs a node")
		}
		n, found := s.liveEngine().NodeByName(name)
		if !found || n.Firmware == nil {
			return nil, fmt.Errorf("%s is not running", name)
		}
		sc, ok := n.Firmware.Backend.(interface {
			Screen() (int, int, int, bool, []byte, bool)
		})
		if !ok {
			return nil, fmt.Errorf("%s is not a board with a display", name)
		}
		width, height, bpp, on, bits, have := sc.Screen()
		if !have {
			return map[string]any{"node": name, "has_screen": false}, nil
		}
		lit := 0
		for _, b := range bits {
			if b != 0 {
				lit++
			}
		}
		// A digest of the whole frame, so a script can tell one screen from the
		// next by identity rather than by a byte count two different frames can
		// share. It is what a wait-for-the-screen-to-change is built on: the
		// count answers "how much is lit", the digest answers "is it the same".
		return map[string]any{"node": name, "has_screen": true,
			"width": width, "height": height, "bpp": bpp, "on": on,
			"lit": lit, "digest": frameDigest(bits)}, nil
	})
}

// frameDigest is a cheap FNV-1a hash of a framebuffer, returned as a hex string
// so a script can compare two screens for identity without carrying the pixels.
// Hex rather than a number because JSON's number is a float64 and a 64-bit hash
// does not survive the round trip whole.
func frameDigest(bits []byte) string {
	const (
		offset = 1469598103934665603
		prime  = 1099511628211
	)
	var h uint64 = offset
	for _, b := range bits {
		h ^= uint64(b)
		h *= prime
	}
	return strconv.FormatUint(h, 16)
}
