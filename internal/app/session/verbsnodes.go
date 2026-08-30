// Firmware, per node and per network: starting it, reporting what came up,
// and changing what a node will run next time.
package session

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerNodeFirmwareVerbs(st *state.Store, s *Sim) {
	st.Handle("firmware.start", func(w *state.World, _ any) (any, error) {
		if s.eng == nil {
			return nil, fmt.Errorf("no network loaded: %w", ErrNoSimulation)
		}
		w.Say("starting firmware on every node")
		s.startFirmware(st, w.Seed)
		return map[string]any{"starting": true}, nil
	})

	st.HandleInternal("firmware.started", func(w *state.World, _ any) (any, error) {
		n := s.firmwareCount()
		// The count, here as well as on the tick.
		//
		// It was only ever written while the engine was stepping, so a mesh
		// that was up but paused reported nought running - which is what the
		// status line said while fifty-six processes answered their consoles.
		w.FirmwareRunning = n
		if w.PendingPlay {
			// The mesh is up, and that is all this reports.
			//
			// It used to start the run here, which meant one press of play
			// produced two things happening a minute apart with nothing
			// asking for the second. Play is what starts a run; this says
			// when pressing it will do something.
			w.PendingPlay = false
			w.Say(fmt.Sprintf("%d nodes running firmware - press play to start the run", n))
			return map[string]any{"running": n, "playing": false}, nil
		}
		w.Say(fmt.Sprintf("%d nodes running firmware", n))
		return map[string]any{"running": n}, nil
	})

	st.HandleInternal("firmware.failed", func(w *state.World, p any) (any, error) {
		msg := soleString(p)
		// A run that was waiting for firmware does not start on a failure. It
		// would advance a clock over a mesh that is not there.
		if w.PendingPlay {
			w.PendingPlay = false
			w.Say("firmware failed, so the run has not started: " + msg)
			return nil, nil
		}
		w.Say("firmware: " + msg)
		return nil, nil
	})

	// firmware.state: how far along starting the mesh is.
	//
	// "nodes" is the nodes that *run* firmware, not every node there is. It
	// used to be every node, and running is a count of processes - so on any
	// scenario holding an SDR observer or an emitter the two could never meet.
	// fife-strict holds one of each, so the shipped fixture reported 56 of 58
	// for ever and every wait built on it hung. Comparing two different
	// populations is the whole of that bug.
	st.Handle("firmware.state", func(w *state.World, _ any) (any, error) {
		runs := 0
		for _, n := range s.nodes {
			if n.Kind.RunsFirmware() {
				runs++
			}
		}
		return map[string]any{
			"running": s.firmwareCount(), "nodes": runs,
			// Every node, for a caller that wants the scenario's size rather
			// than the part of it that boots.
			"total":    len(w.Nodes),
			"starting": s.starting.Load(),
		}, nil
	})

	st.Handle("nodes.stats", func(w *state.World, _ any) (any, error) {
		// Also on demand, because a paused simulation still costs memory and
		// somebody looking at the node view has usually just paused it.
		w.Stats = s.nodeStats(w.Events)
		// And the rows, not only how many there are.
		//
		// It answered with a count and put the rows in the snapshot, where
		// only a panel could reach them - so from outside the window there
		// was no way to ask whether a node was running, which is the first
		// thing any script wants to know and the thing every wait is built
		// on. The count stays for whoever is already reading it.
		return map[string]any{"nodes": len(w.Stats), "stats": statRows(w.Stats)}, nil
	})

	st.Handle("node.stop", func(w *state.World, p any) (any, error) {
		name := soleString(p)
		if err := s.stopNode(name); err != nil {
			return nil, err
		}
		w.Stats = s.nodeStats(w.Events)
		w.Say("stopped " + name)
		return map[string]any{"stopped": name}, nil
	})

	st.Handle("node.start", func(w *state.World, p any) (any, error) {
		name := soleString(p)
		if err := s.startNode(context.Background(), name, w.Seed); err != nil {
			return nil, err
		}
		w.Stats = s.nodeStats(w.Events)
		w.Say("started " + name)
		return map[string]any{"started": name}, nil
	})

	// board.press: hold one of a board's own buttons down, or let it go.
	//
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

	// board.key: type one character at the board's own keyboard.
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

	// board.touch: put a finger on the panel, or take it off.
	st.Handle("board.touch", func(w *state.World, p any) (any, error) {
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

	// board.screen: what the board's own display is showing, as numbers.
	//
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

	st.Handle("node.set_firmware", func(w *state.World, p any) (any, error) {
		m, _ := p.(map[string]any)
		name, _ := m["node"].(string)
		version, _ := m["version"].(string)
		// The board and the role travel with the version, because a board
		// image is not a build on its own: it is that image for that hardware.
		// Absent means a build for this machine, which is what every node ran
		// before board images could be chosen here at all.
		b := Build{Version: version}
		b.Board, _ = m["board"].(string)
		b.Role, _ = m["role"].(string)
		// Applied, not just recorded: stop, provision, start. Firmware is
		// chosen when a node launches, so setting it on a running node changes
		// nothing until something restarts it.
		s.Reflash(context.Background(), st, name, b, w.Seed)
		// A different build may insist on storage where the last one did not,
		// so what the node's slot will hold has just changed.
		s.publishCards(w)
		w.Say(name + ": changing to " + b.Describe())
		return map[string]any{"node": name, "version": version,
			"board": b.Board, "role": b.Role}, nil
	})

	st.HandleInternal("node.reflashed", func(w *state.World, p any) (any, error) {
		msg := soleString(p)
		w.Stats = s.nodeStats(w.Events)
		// The node's own window reads the node list rather than the stats, so
		// a change that stopped at the stats showed in the table and nowhere
		// else. Name is the first word of the message the reflash sent.
		if name, _, ok := strings.Cut(msg, " "); ok {
			s.refreshNodeBuild(w, name)
		}
		w.Say(msg)
		return nil, nil
	})

	st.HandleInternal("node.reflash_failed", func(w *state.World, p any) (any, error) {
		msg := soleString(p)
		w.Stats = s.nodeStats(w.Events)
		w.Say("firmware change failed: " + msg)
		return nil, nil
	})

	st.Handle("node.set_firmware_only", func(w *state.World, p any) (any, error) {
		m, _ := p.(map[string]any)
		name, _ := m["node"].(string)
		version, _ := m["version"].(string)
		b := Build{Version: version}
		b.Board, _ = m["board"].(string)
		b.Role, _ = m["role"].(string)
		if err := s.setFirmware(name, b); err != nil {
			return nil, err
		}
		s.refreshNodeBuild(w, name)
		// Not applied until the node restarts, and said so rather than left to
		// be discovered: firmware is chosen at launch, and a version that
		// changes nothing until something else happens is the kind of control
		// somebody presses twice and then distrusts.
		w.Say(name + " will run " + b.Describe() + " when it next starts")
		return map[string]any{"node": name, "version": version,
			"board": b.Board, "role": b.Role}, nil
	})

	st.Handle("node.provisioning", func(w *state.World, p any) (any, error) {
		name := soleString(p)
		lines, err := s.provisioningFor(name)
		if err != nil {
			return nil, err
		}
		w.Provisioning, w.ProvisioningNode = lines, name
		var cmds []string
		for _, l := range lines {
			if !l.Comment {
				cmds = append(cmds, l.Command)
			}
		}
		return map[string]any{"node": name, "commands": cmds}, nil
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
