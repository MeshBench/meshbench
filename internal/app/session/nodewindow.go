// Opening one node's own window - the thing people put on a second monitor.
//
// Beside the firmware window's verb rather than in ui.go, which holds the
// interface verbs and was at the length limit: this one is about a node and
// only incidentally about a window.
package session

import (
	"github.com/MeshBench/meshbench/internal/app/control"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerNodeWindow(st *state.Store, s *Sim) {
	// node.window: the thing people put on a second monitor.
	st.Handle("node.window", func(w *state.World, p any) (any, error) {
		if err := s.needUI(); err != nil {
			return nil, err
		}
		// Both halves, in the order that keeps each one's answer. Reading the
		// sole value and then overwriting it with m["node"] whatever that held
		// meant {"tab": "Radio"} emptied the name it had just read, and the
		// window somebody double-clicked was refused for a node called "".
		name := primaryString(p, "node")
		if _, found := findNode(w.Nodes, name); !found {
			return nil, noSuchNode(name)
		}
		tab, _ := namedField(p, "tab")
		shown, err := s.ui.OpenNodeWindow(name, tab)
		if err != nil {
			return nil, control.WithCode(control.BadParams, err)
		}
		return map[string]any{"node": name, "tab": shown}, nil
	})
}
