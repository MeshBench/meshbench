// The companion bench: serving a node over TCP or a PTY so a real client can
// talk to a simulated radio.
package session

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerBenchVerbs(st *state.Store, s *Sim) {
	st.HandleSpec("bench.serve", state.Spec{
		What: "open a real endpoint onto one simulated node, so an unmodified " +
			"companion client on this machine or another can talk to it as if it " +
			"were hardware",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString,
				What: "the node to serve; absent takes the first companion in the " +
					"scenario, because an endpoint is what is wanted before a node " +
					"has been chosen"},
			{Name: "kind", Type: state.ParamString,
				What: `"serial" for a pseudo-terminal a serial client opens; ` +
					"anything else, absent included, listens on TCP on every " +
					"interface and a port the operating system picks"},
		},
		Returns: []string{"node", "addr"},
		Answers: "Serving takes the port off the workbench, so a companion " +
			"session on that node is released rather than shared. A second call " +
			"for the same node replaces the listener rather than adding one.",
		Example: &state.Example{
			Params: map[string]any{"node": "West Lomond", "kind": "tcp"},
			What:   "put a node on a TCP port for a real client",
		},
	}, func(w *state.World, p any) (any, error) {
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
		// serve released any claim the workbench held on this node, so the
		// panel has to stop saying "connected".
		s.publishCompanions(w)
		w.Say(ep.Node + " is at " + ep.Addr)
		return map[string]any{"node": ep.Node, "addr": ep.Addr}, nil
	})

	st.HandleSpec("bench.drop", state.Spec{
		What: "take a served node's port back, or with no node named cut every " +
			"attached client loose while leaving the listeners open",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString,
				What: "the node to stop serving, whether or not a client is on " +
					"it; absent drops the clients instead and stops serving nothing"},
		},
		Returns: []string{"dropped"},
		Answers: "`dropped` counts what was closed, and zero is a normal answer: " +
			"named a node nothing was serving, or asked to drop clients when none " +
			"were attached.",
		Example: &state.Example{
			Params:   map[string]any{"node": "West Lomond"},
			What:     "stop serving one node",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
		// With a node: stop serving that node, attached or not. A listener
		// still waiting for its first client is exactly what "stop serving"
		// is pressed at, and closing only attached links made that press a
		// silent nothing - the port stayed served and Connect stayed refused.
		if m, ok := p.(map[string]any); ok {
			if name, _ := m["node"].(string); name != "" {
				n := s.stopServing(name)
				w.Endpoints = s.endpoints()
				s.publishCompanions(w)
				if n == 0 {
					w.Say(name + " was not being served")
				} else {
					w.Say("stopped serving " + name)
				}
				return map[string]any{"dropped": n}, nil
			}
		}
		n := s.dropClients()
		w.Endpoints = s.endpoints()
		s.publishCompanions(w)
		w.Say(fmt.Sprintf("dropped %d client connection(s)", n))
		return map[string]any{"dropped": n}, nil
	})

	st.HandleSpec("bench.stray", state.Spec{
		What: "hand a companion a frame it was never going to be sent, so what " +
			"the decoder does with rubbish can be watched rather than assumed",
		Returns: []string{"at"},
		Answers: "It injects at the first companion in the scenario, and names " +
			"which that was. Refused where there is no run, or where the scenario " +
			"holds no companion at all.",
		Example: &state.Example{
			Params: map[string]any{}, What: "put a bad frame at a companion",
		},
	}, func(w *state.World, _ any) (any, error) {
		if s.eng == nil {
			return nil, ErrNoSimulation
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

	// Whether a client is attached changes without anything asking, so the
	// panel refreshes rather than assuming what it drew is still true.
	st.HandleSpec("bench.refresh", state.Spec{
		What: "look again at which endpoints are open and which have a client on " +
			"them, because a client attaching or leaving tells this session nothing",
		Answers: "It answers with nothing at all. The endpoints go into the " +
			"published state, which is where a panel reads them.",
		Example: &state.Example{
			Params: map[string]any{}, What: "see whether a client has attached",
			Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
		w.Endpoints = s.endpoints()
		return nil, nil
	})
}
