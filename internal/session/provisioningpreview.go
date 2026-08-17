// The resolved script for one node - what will actually be sent, and which
// rule asked for each line. With more than one rule in play a node's script
// is no longer something an operator can work out by reading the panel, so
// this is what the preview shows instead.
package session

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/provision"
)

// previewFor is the active rules resolved against one node's readback, in
// the shape the panel draws - shared by the preview verb and node.provisioning
// so the two cannot show two different answers to "what will this node be
// sent".
func (s *Sim) previewFor(ns provision.NodeState) []state.ProvisionResolvedLine {
	resolved := provision.Resolve(s.activeRules(), ns)
	lines := make([]state.ProvisionResolvedLine, len(resolved))
	for i, r := range resolved {
		lines[i] = state.ProvisionResolvedLine{Command: r.Command, RuleName: r.RuleName}
	}
	return lines
}

func registerProvisioningPreview(st *state.Store, s *Sim) {
	// provisioning.preview: resolved against the last readback, not a fresh
	// one - a verb runs on the store's own goroutine and must not block it
	// waiting for firmware to answer, and the readback is already there to
	// resolve against once a run has happened once.
	st.Handle("provisioning.preview", func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "node")
		if name == "" {
			return nil, fmt.Errorf("provisioning.preview needs a node")
		}
		ns, ok := s.readback[name]
		if !ok {
			w.ProvisioningPreviewNode, w.ProvisioningPreview = name, nil
			return map[string]any{
				"node": name, "lines": 0,
				"note": "no readback yet for this node - start the mesh, or send " +
					"'read now' first",
			}, nil
		}
		lines := s.previewFor(ns)
		w.ProvisioningPreviewNode, w.ProvisioningPreview = name, lines
		return map[string]any{"node": name, "lines": len(lines)}, nil
	})

	// provisioning.readback: reads every targeted node without sending
	// anything they are told to become - the "read now" the preview note
	// above points to, useful on its own to see what a mesh currently holds.
	st.Handle("provisioning.readback", func(w *state.World, p any) (any, error) {
		if s.eng == nil {
			return nil, fmt.Errorf("no network loaded")
		}
		only := stringSetField(p, "nodes")
		var names []string
		for n := range only {
			names = append(names, n)
		}
		if s.starting.Swap(true) {
			return nil, fmt.Errorf("firmware is already starting or provisioning")
		}
		go func() {
			defer s.starting.Store(false)
			s.runReadbackOnly(st, names)
		}()
		return map[string]any{"started": true}, nil
	})
}
