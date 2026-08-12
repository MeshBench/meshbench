// Control verbs, registered on the store.
//
// The parity test in 12.9 is generated from what is registered here rather
// than from a list in a document, which was already wrong by three verbs.
package main

import (
	"fmt"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
)

// registerVerbs wires the control verbs onto the store. Only the few the new
// UI needs so far; the rest arrive as their panels do, and the parity test in
// 12.9 is generated from what is registered here.
func registerVerbs(st *state.Store) {
	st.Handle("project.open", func(w *state.World, p any) (any, error) {
		path, _ := p.(string)
		nodes, err := loadFixture(path)
		if err != nil {
			return nil, err
		}
		w.Nodes = nodes
		w.Seed = 9001
		w.Say(fmt.Sprintf("opened %s: %d nodes", path, len(nodes)))
		return map[string]any{"opened": path, "nodes": len(nodes)}, nil
	})
	st.Handle("sim.play", func(w *state.World, _ any) (any, error) {
		w.Playing = true
		w.Say("playing")
		return map[string]any{"playing": true}, nil
	})
	st.Handle("sim.pause", func(w *state.World, _ any) (any, error) {
		w.Playing = false
		w.Say("paused")
		return map[string]any{"playing": false}, nil
	})
	st.Handle("nodes.select", func(w *state.World, p any) (any, error) {
		name, _ := p.(string)
		for i := range w.Nodes {
			w.Nodes[i].Selected = w.Nodes[i].Name == name
		}
		return map[string]any{"selected": name}, nil
	})
	st.Handle("session.describe", func(w *state.World, _ any) (any, error) {
		return map[string]any{
			"nodes": len(w.Nodes), "seed": w.Seed, "now_ms": w.NowMs,
			"playing": w.Playing,
		}, nil
	})
}
