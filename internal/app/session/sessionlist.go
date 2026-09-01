// session.list: the workbenches running on this machine.
//
// A script driving one session may need to know what else is up - a soak run
// beside the workbench somebody is watching, two jobs on one CI runner - and
// until there was a registry there was no way to ask. The registry is in
// internal/app/control, which is also what a client reads directly when it has
// no session to ask yet; this is the same list, seen from inside one.
package session

import (
	"sync/atomic"

	"github.com/MeshBench/meshbench/internal/app/control"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/app/version"
)

// answeringAt is where this session's control socket answers, so that its own
// row can be told from everybody else's.
//
// Atomic because it is written when the socket opens and read on the store's
// goroutine when the verb runs. Empty in a session with no socket at all,
// which is most tests: then every row is somebody else's and every row is
// probed, which is exactly right.
var answeringAt atomic.Value

func setAnsweringAt(socket string) { answeringAt.Store(socket) }

func answering() string {
	s, _ := answeringAt.Load().(string)
	return s
}

func registerSessionList(st *state.Store) {
	st.HandleSpec("session.list", state.Spec{
		What:    "list the workbenches running on this machine, this one included",
		Returns: []string{"sessions", "count"},
	}, func(w *state.World, _ any) (any, error) {
		rows, err := control.Sessions(answering())
		if err != nil {
			return nil, err
		}
		for i := range rows {
			if rows[i].Self {
				// The one row control cannot fill in: asking ourselves over
				// the socket would wait for a reply this goroutine is the one
				// that owes. The answer is here already, and here it cannot
				// be stale either.
				rows[i].Detail = control.Detail{
					Version: version.Detail(), Mode: Mode,
					Project: w.Project, Nodes: len(w.Nodes),
				}
			}
		}
		return map[string]any{"sessions": rows, "count": len(rows)}, nil
	})
}
