// The companion bench: serving a node over TCP or a PTY so a real client can
// talk to a simulated radio.
package session

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/gui/state"
)

func registerBenchVerbs(st *state.Store, s *Sim) {
	st.Handle("bench.serve", func(w *state.World, p any) (any, error) {
		m, _ := p.(map[string]any)
		name, _ := m["node"].(string)
		kind, _ := m["kind"].(string)
		if name == "" {
			// The first companion, because "give me an endpoint" is what
			// somebody means before they have chosen one.
			for i := range w.Nodes {
				if w.Nodes[i].Kind == "companion" {
					name = w.Nodes[i].Name
					break
				}
			}
		}
		ep, err := s.serve(name, kind)
		if err != nil {
			w.Say("could not serve " + name + ": " + err.Error())
			return nil, err
		}
		w.Endpoints = s.endpoints()
		w.Say(ep.Node + " is at " + ep.Addr)
		return map[string]any{"node": ep.Node, "addr": ep.Addr}, nil
	})

	st.Handle("bench.drop", func(w *state.World, _ any) (any, error) {
		n := s.dropClients()
		w.Endpoints = s.endpoints()
		w.Say(fmt.Sprintf("dropped %d client connection(s)", n))
		return map[string]any{"dropped": n}, nil
	})

	st.Handle("bench.stray", func(w *state.World, _ any) (any, error) {
		if s.eng == nil {
			return nil, fmt.Errorf("no simulation")
		}
		for i := range w.Nodes {
			if w.Nodes[i].Kind == "companion" {
				s.eng.Inject(i, []byte("msim-stray"))
				w.Say("injected an unexpected frame at " + w.Nodes[i].Name)
				return map[string]any{"at": w.Nodes[i].Name}, nil
			}
		}
		return nil, fmt.Errorf("no companion to inject at")
	})

	st.Handle("bench.refresh", func(w *state.World, _ any) (any, error) {
		// Whether a client is attached changes without anything asking, so
		// the panel refreshes rather than assuming what it drew is still true.
		w.Endpoints = s.endpoints()
		return nil, nil
	})
}
