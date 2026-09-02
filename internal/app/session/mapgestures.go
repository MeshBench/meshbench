// The verbs the map's own gestures go through: choosing nodes, dragging one,
// and originating a packet at one.
//
// A pointer gesture is not allowed to write to the world directly, so a box
// drag, a click and a drop all arrive here as the same verbs a script would
// call. That is what makes them worth guarding carefully: every one of them
// takes a node name or a coordinate from outside, and each used to have an
// answer for a parameter it could not understand.
package session

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerMapGestures(st *state.Store, s *Sim) {
	st.Handle("nodes.select", func(w *state.World, p any) (any, error) {
		name := primaryString(p, "node")
		for i := range w.Nodes {
			w.Nodes[i].Selected = w.Nodes[i].Name == name
		}
		return map[string]any{"selected": name}, nil
	})

	st.Handle("nodes.select_many", func(w *state.World, p any) (any, error) {
		// Several shapes, because a selection arrives from a box drag as a list
		// and from the control socket as a name or a JSON list, and a caller
		// should not have to know which the interface happens to use. A shape
		// outside that set is refused: the loop below writes every node's
		// Selected, so a parameter nothing recognised used to deselect the whole
		// network and answer as though it had selected something.
		names, err := namesOf("nodes.select_many", p)
		if err != nil {
			return nil, err
		}
		if err := unknownNames("nodes.select_many", w.Nodes, names); err != nil {
			return nil, err
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
		names, err := namesOf("nodes.add_to_selection", p)
		if err != nil {
			return nil, err
		}
		if err := unknownNames("nodes.add_to_selection", w.Nodes, names); err != nil {
			return nil, err
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
		// All three read out rather than asserted, because an unchecked
		// assertion here is a teleport: a mistyped or missing coordinate came
		// back as zero, the node went to the Gulf of Guinea, and the move
		// reported the position it had just invented as though it had been
		// asked for.
		m, isObject := p.(map[string]any)
		if !isObject {
			return nil, badParams("nodes.move takes a node and a position: " +
				`{"node": ..., "lat": ..., "lon": ...}`)
		}
		// "node" is what every other verb that acts on an existing node calls
		// it, and this one asked for "name" alone: a script that had learnt the
		// surface everywhere else was refused here and nowhere else. Both are
		// read, because the older spelling is in saved scripts and in the
		// documentation, and there is no ambiguity between them.
		name, _ := m["node"].(string)
		if name == "" {
			name, _ = m["name"].(string)
		}
		if name == "" {
			return nil, badParams("nodes.move needs a node: which node to move")
		}
		lat, err := requiredNum("nodes.move", "lat", p, -90, 90)
		if err != nil {
			return nil, err
		}
		lon, err := requiredNum("nodes.move", "lon", p, -180, 180)
		if err != nil {
			return nil, err
		}
		for i := range w.Nodes {
			if w.Nodes[i].Name == name {
				w.Nodes[i].Lat, w.Nodes[i].Lon = lat, lon
				// The physics moves with the marker: cached losses for this
				// node are forgotten, so an attached SDR client hears the
				// new position on the next window.
				if s.eng != nil {
					s.eng.SetNodePosition(i, lat, lon)
				}
				// Both spellings in the reply too, so a caller reading either
				// one back gets the node it moved.
				return map[string]any{
					"node": name, "name": name, "lat": lat, "lon": lon,
				}, nil
			}
		}
		return nil, noSuchNode(name)
	})

	st.Handle("sim.inject", func(w *state.World, p any) (any, error) {
		// A network with no nodes has nowhere to originate from. It used to
		// be unreachable - every session began with a fixture - and starting
		// a blank network made it a state somebody can be in, where this
		// indexed an empty slice and took the process with it.
		if len(w.Nodes) == 0 {
			return nil, fmt.Errorf("no nodes to originate from - place one first")
		}
		// The name is resolved before the engine is asked for, so that a name
		// this network has not got is reported as itself whatever state the
		// session is in. Told "no simulation" first, somebody fixes the
		// simulation and then meets the same typo again.
		at := 0
		if name := primaryString(p, "node"); name != "" {
			// Refused rather than fallen through to node 0. A name that matched
			// nothing used to originate the packet at whichever node happened to
			// be first and report that as success, so a typo in a script moved
			// the transmitter and nothing said so.
			at = -1
			for i := range w.Nodes {
				if w.Nodes[i].Name == name {
					at = i
				}
			}
			if at < 0 {
				return nil, unknownNames("sim.inject", w.Nodes, []string{name})
			}
		} else {
			for i := range w.Nodes {
				if w.Nodes[i].Selected {
					at = i
					break
				}
			}
		}
		// Originating a packet without firmware on the node. The engine
		// delivers to everything in range regardless, so this exercises the
		// radio model and the map's traffic layer; what it does not exercise
		// is relaying, which is a firmware behaviour and needs a firmware.
		if s.eng == nil {
			return nil, ErrNoSimulation
		}
		s.eng.Inject(at, []byte("msim-map-trace"))
		w.Say("injected a packet at " + w.Nodes[at].Name)
		return map[string]any{"at": w.Nodes[at].Name}, nil
	})
}
