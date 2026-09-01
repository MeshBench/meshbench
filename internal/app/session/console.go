// Talking to one node's firmware.
//
// The Gio build had no firmware console at all. Its "Console" panel is the
// event log filtered to the selected node, which answers "what did this node
// send" but not "what did this node say" - and the second question is the one
// somebody asks when a node is not doing what its configuration claims.
package session

import (
	"context"
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware/console"
)

// consoleFor returns the buffer attached to a node's firmware, attaching one
// on first use.
//
// Keyed on the bridge as well as the name, so a node whose firmware has been
// restarted gets a fresh buffer rather than one still holding the dead
// process's output - which reads as a node that has stopped talking.
func (s *Sim) consoleFor(name string) (*console.Buf, error) {
	if s.eng == nil {
		return nil, fmt.Errorf("no network loaded")
	}
	n, ok := s.eng.NodeByName(name)
	if !ok {
		return nil, noSuchNode(name)
	}
	if n.Firmware == nil {
		return nil, fmt.Errorf("%s runs no firmware, so it has no console", name)
	}
	if s.consoles == nil {
		s.consoles = map[string]*console.Buf{}
	}
	buf, seen := s.consoles[name]
	if !seen || buf.Bridge != n.Firmware.Bridge {
		buf = &console.Buf{Bridge: n.Firmware.Bridge}
		// The clock at the moment it is attached, not at the next step: a
		// buffer created by the very command being typed would otherwise
		// stamp that line 0.000 whatever the run had reached.
		buf.SetNow(s.eng.NowMs())
		n.Firmware.Bridge.Console(buf)
		s.consoles[name] = buf
	}
	return buf, nil
}

func registerConsole(st *state.Store, s *Sim) {
	st.HandleSpec("console.type", state.Spec{
		What: "run a line at a node's own firmware console, which answers what " +
			"the node says rather than what it sent, and is the question asked " +
			"when a node is not behaving as its configuration claims",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true,
				What: "the node whose console to type at; refused when it is " +
					"absent, unknown, or runs no firmware"},
			{Name: "command", Type: state.ParamString, Required: true,
				What: "the line to type, sent with a carriage return and newline " +
					"after it; refused when it is absent or empty"},
		},
		Returns: []string{"node", "sent", "note"},
		Answers: "The node's reply is not in this answer: it lands in the " +
			"scrollback when the engine next steps, and is read with " +
			"`console.read`. On a paused clock this steps the engine sixty times " +
			"so the reply arrives anyway, but while a sweep owns the clock the " +
			"replies come back empty.",
		Example: &state.Example{
			Params: map[string]any{"node": "West Lomond", "command": "get name"},
			What:   "ask a node what it believes it is called",
		},
	}, func(w *state.World, p any) (any, error) {
		var name, cmd string
		if m, ok := p.(map[string]any); ok {
			name, _ = m["node"].(string)
			cmd, _ = m["command"].(string)
		}
		if name == "" || cmd == "" {
			return nil, fmt.Errorf("console.type needs a node and a command")
		}
		buf, err := s.consoleFor(name)
		if err != nil {
			return nil, err
		}
		mark := buf.Mark()
		buf.Echo(cmd)
		n, _ := s.eng.NodeByName(name)
		if err := n.Firmware.Bridge.Type([]byte(cmd + "\r\n")); err != nil {
			return nil, err
		}
		// The reply arrives when the engine next steps. Playing, the ticker
		// does that within milliseconds and the tick republishes the console.
		// Paused, nothing would ever step - so a typed command answered with
		// silence until somebody pressed play, which reads as a console that
		// does not answer. The old workbench steps sixty after typing; the
		// same here, but only when paused: waiting for the ticker from the
		// ticker's own goroutine is waiting for yourself, and with
		// fifty-eight firmware processes it blocked every verb for seconds.
		if !w.Playing {
			for i := 0; i < 60; i++ {
				_ = s.eng.Step(context.Background())
			}
			w.NowMs = s.eng.NowMs()
		}
		w.Console, w.ConsoleNode = buf.Snapshot(), name
		_ = mark
		return map[string]any{
			"node": name, "sent": cmd,
			"note": "the reply lands when the engine next steps; read it with console.read",
		}, nil
	})

	st.HandleSpec("console.read", state.Spec{
		What: "read back what a node's firmware has said, which is where the " +
			"reply to anything typed at it arrives",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true, Primary: true,
				What: "the node whose scrollback to read; refused when it is " +
					"absent, unknown, or runs no firmware"},
		},
		Returns: []string{"node", "lines", "tail"},
		Answers: "`lines` is how many the node has said since its console was " +
			"attached and `tail` is the last two hundred of them, so the two " +
			"disagree on a node that has been running a while.",
		Example: &state.Example{
			Params: "West Lomond", What: "read what a node has been saying",
		},
	}, func(w *state.World, p any) (any, error) {
		name := soleString(p)
		if m, ok := p.(map[string]any); ok {
			name, _ = m["node"].(string)
		}
		buf, err := s.consoleFor(name)
		if err != nil {
			return nil, err
		}
		w.Console, w.ConsoleNode = buf.Snapshot(), name
		// The scrollback itself, not a count of it.
		//
		// It reported only how many lines there were, which is exactly the
		// thing a caller already knows it does not know. The tail rather than
		// all of it: a node that has been running for an hour has thousands,
		// and nobody reads the first one.
		tail := w.Console
		if n := 200; len(tail) > n {
			tail = tail[len(tail)-n:]
		}
		return map[string]any{
			"node": name, "lines": len(w.Console), "tail": tail,
		}, nil
	})
}
