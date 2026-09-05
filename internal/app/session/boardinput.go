// Reaching a board's own controls: its buttons, its keyboard, its touch panel,
// and what its display is showing as a result.
//
// The pair is what makes any of them worth having. A control that reaches the
// hardware silently is indistinguishable from one that does not reach it at
// all, so every one of these has board.screen beside it: press, then ask
// whether anything on the panel changed.
package session

import (
	"context"
	"fmt"
	"strconv"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerBoardInput(st *state.Store, s *Sim) {
	// Held rather than clicked, because the firmware behind these pins cares:
	// MeshCore wakes a sleeping display on a press and powers the board off on
	// a long one, and a verb that could only produce a tap could reach neither.
	st.Handle("board.press", func(w *state.World, p any) (any, error) {
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

	st.Handle("board.key", func(w *state.World, p any) (any, error) {
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

	st.Handle("board.touch", func(w *state.World, p any) (any, error) {
		m, _ := p.(map[string]any)
		name, _ := m["node"].(string)
		xf, okX := namedNum(p, "x")
		yf, okY := namedNum(p, "y")
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
	st.Handle("board.screen", func(w *state.World, p any) (any, error) {
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

// board.reset restarts one board, which is what its own reset button does.
//
// Stopped and started rather than anything cleverer, because that is what a
// reset is to everything downstream: the machine is torn down and built again
// from the same flash, so whatever the firmware wrote survives and whatever it
// held in memory does not. The alternative - poking the guest's reset line -
// would leave our own models holding state the guest no longer has, which is a
// board that half-rebooted and is worse than one that did not.
func registerBoardReset(st *state.Store, s *Sim) {
	st.Handle("board.reset", func(w *state.World, p any) (any, error) {
		name := primaryString(p, "node")
		if _, found := findNode(w.Nodes, name); !found {
			return nil, noSuchNode(name)
		}
		if err := s.stopNode(name); err != nil {
			return nil, err
		}
		if err := s.startNode(context.Background(), name, w.Seed); err != nil {
			return nil, err
		}
		w.Stats = s.nodeStats(w.Events)
		// Said, because a board that comes back in two seconds looks like a
		// board that did nothing.
		w.Say("reset " + name)
		return map[string]any{"reset": name}, nil
	})
}
