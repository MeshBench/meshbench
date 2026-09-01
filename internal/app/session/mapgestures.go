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
	st.HandleSpec("nodes.select", state.Spec{
		What: "make one node the selection, which is what the verbs that act on " +
			"\"the selected node\" - sim.inject among them - go on to find",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Primary: true,
				What: "which node; a name this network has not got is not " +
					"refused here, and clears the selection, as an empty name does"},
		},
		Returns: []string{"selected"},
		Answers: "`selected` is the name that was asked for, whether or not a " +
			"node answers to it: this is the click a map sends, and it sets " +
			"every node's selected flag from that one name.",
		Example: &state.Example{
			Params: "West Lomond", What: "select one node",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
		name := soleString(p)
		for i := range w.Nodes {
			w.Nodes[i].Selected = w.Nodes[i].Name == name
		}
		return map[string]any{"selected": name}, nil
	})

	st.HandleSpec("nodes.select_many", state.Spec{
		What: "replace the selection with a set of nodes, which is what a box " +
			"drag on the map amounts to and what a script does before any verb " +
			"that acts on a selection",
		Params: []state.Param{
			{Name: "names", Type: state.ParamArray, Primary: true,
				What: "the nodes to select, as a list, one name, or " +
					`{"names": [...]}; a name this network has not got refuses ` +
					"the whole call, and no names at all clears the selection"},
		},
		Returns: []string{"selected"},
		Answers: "Every other node is deselected, so this is a replacement " +
			"rather than an addition; nodes.add_to_selection is the addition.",
		Example: &state.Example{
			Params:   map[string]any{"names": []any{"West Lomond", "Dunfermline"}},
			What:     "select two nodes at once",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
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

	st.HandleSpec("nodes.add_to_selection", state.Spec{
		What: "add nodes to whatever is already selected, which is the " +
			"shift-drag, and the way a selection is built up out of several " +
			"passes over the map",
		Params: []state.Param{
			{Name: "names", Type: state.ParamArray, Primary: true,
				What: "the nodes to add, as a list, one name, or " +
					`{"names": [...]}; a name this network has not got refuses ` +
					"the whole call, and no names at all adds nothing and leaves " +
					"the selection as it was"},
		},
		Returns: []string{"added"},
		Answers: "`added` counts the nodes matched, not the nodes newly " +
			"selected: one that was already in the selection is counted again.",
		Example: &state.Example{
			Params:   map[string]any{"names": []any{"Dunfermline"}},
			What:     "add one more node to the selection",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
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

	st.HandleSpec("nodes.move", state.Spec{
		What: "put a node at a position and move its physics with its marker, " +
			"forgetting the losses cached for it so the next window an attached " +
			"SDR client hears is the one from where it now stands",
		Params: []state.Param{
			{Name: "name", Type: state.ParamString, Required: true,
				What: "which node moves; absent, blank or unknown is refused"},
			{Name: "lat", Type: state.ParamNumber, Required: true,
				What: "degrees north, minus 90 to 90; absent or outside that is " +
					"refused rather than read as nought, which used to put the " +
					"node in the Gulf of Guinea and report it as asked for"},
			{Name: "lon", Type: state.ParamNumber, Required: true,
				What: "degrees east, minus 180 to 180; absent or outside that " +
					"is refused"},
		},
		Returns: []string{"name", "lat", "lon"},
		Example: &state.Example{
			Params:   map[string]any{"name": "West Lomond", "lat": 56.25, "lon": -3.29},
			What:     "move a node onto the hill it is named after",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
		// All three read out rather than asserted, because an unchecked
		// assertion here is a teleport: a mistyped or missing coordinate came
		// back as zero, the node went to the Gulf of Guinea, and the move
		// reported the position it had just invented as though it had been
		// asked for.
		m, isObject := p.(map[string]any)
		if !isObject {
			return nil, badParams("nodes.move takes a node and a position: " +
				`{"name": ..., "lat": ..., "lon": ...}`)
		}
		name, _ := m["name"].(string)
		if name == "" {
			return nil, badParams("nodes.move needs a name: which node to move")
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
				return map[string]any{"name": name, "lat": lat, "lon": lon}, nil
			}
		}
		return nil, noSuchNode(name)
	})

	st.HandleSpec("sim.inject", state.Spec{
		What: "put one packet on the air from a node, to exercise the radio " +
			"model and the traffic layer without firmware behind it",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Primary: true,
				What: "which node transmits; absent means the selected one, " +
					"and a name this network has not got is refused rather " +
					"than falling through to the first node"},
		},
		Returns: []string{"at"},
		Answers: "Nothing relays what this originates: relaying is a firmware " +
			"behaviour, and this packet has no firmware behind it.",
		Example: &state.Example{
			Params: "West Lomond", What: "transmit from one named node",
			Runnable: false,
		},
	}, func(w *state.World, p any) (any, error) {
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
		if name := soleString(p); name != "" {
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
