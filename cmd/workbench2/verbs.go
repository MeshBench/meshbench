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
	st.Handle("nodes.select_many", func(w *state.World, p any) (any, error) {
		// Two shapes, because a selection arrives from a box drag as a list
		// and from the control socket as a name, and a caller should not have
		// to know which the interface happens to use.
		var names []string
		switch v := p.(type) {
		case []string:
			names = v
		case string:
			names = []string{v}
		}
		want := map[string]bool{}
		for _, n := range names {
			want[n] = true
		}
		for i := range w.Nodes {
			w.Nodes[i].Selected = want[w.Nodes[i].Name]
		}
		return map[string]any{"selected": names}, nil
	})
	st.Handle("nodes.add_to_selection", func(w *state.World, p any) (any, error) {
		var names []string
		switch v := p.(type) {
		case []string:
			names = v
		case string:
			names = []string{v}
		}
		n := 0
		for _, name := range names {
			for i := range w.Nodes {
				if w.Nodes[i].Name == name {
					w.Nodes[i].Selected = true
					n++
				}
			}
		}
		return map[string]any{"added": n}, nil
	})
	st.Handle("nodes.move", func(w *state.World, p any) (any, error) {
		m, _ := p.(map[string]any)
		name, _ := m["name"].(string)
		lat, _ := m["lat"].(float64)
		lon, _ := m["lon"].(float64)
		for i := range w.Nodes {
			if w.Nodes[i].Name == name {
				w.Nodes[i].Lat, w.Nodes[i].Lon = lat, lon
				return map[string]any{"name": name, "lat": lat, "lon": lon}, nil
			}
		}
		return nil, fmt.Errorf("no node named %q", name)
	})
	st.Handle("session.describe", func(w *state.World, _ any) (any, error) {
		return map[string]any{
			"nodes": len(w.Nodes), "seed": w.Seed, "now_ms": w.NowMs,
			"playing": w.Playing,
		}, nil
	})
}
