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
	"time"

	"github.com/A13xB0/meshcoresim/internal/console"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
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
		return nil, fmt.Errorf("no node named %q", name)
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
		n.Firmware.Bridge.Console(buf)
		s.consoles[name] = buf
	}
	return buf, nil
}

func registerConsole(st *state.Store, s *Sim) {
	// console.type: send a line and report what came back.
	//
	// The documented caveat, said here rather than in a document: replies do
	// not arrive while a sweep owns the clock, because the firmware only
	// speaks when the engine steps it.
	st.Handle("console.type", func(w *state.World, p any) (any, error) {
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
		// Step the engine so the firmware gets a chance to answer. Without
		// this the reply arrives after the caller has already been told there
		// was none.
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			_ = s.eng.Step(context.Background())
			if len(buf.LinesSince(mark)) > 1 {
				break
			}
		}
		w.Console, w.ConsoleNode = buf.Snapshot(), name
		return map[string]any{"node": name, "reply": buf.LinesSince(mark)}, nil
	})

	// console.read: the scrollback, for a window that is drawing it.
	st.Handle("console.read", func(w *state.World, p any) (any, error) {
		name, _ := p.(string)
		if m, ok := p.(map[string]any); ok {
			name, _ = m["node"].(string)
		}
		buf, err := s.consoleFor(name)
		if err != nil {
			return nil, err
		}
		w.Console, w.ConsoleNode = buf.Snapshot(), name
		return map[string]any{"node": name, "lines": len(w.Console)}, nil
	})
}
