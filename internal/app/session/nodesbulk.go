// Changing several nodes at once, and changing what one of them is.
//
// nodes.delete took one name, so trimming a fixture to two nodes meant
// fifty-six calls - each rebuilding the seeded scenario and starting a warm
// that the next one cancelled. Correct, and minutes of it. These do the work
// once.
package session

import (
	"fmt"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/control"
	"github.com/MeshBench/meshbench/internal/app/state"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

func registerNodesBulk(st *state.Store, s *Sim) {
	st.HandleSpec("nodes.delete_many", state.Spec{
		What: "remove a set of nodes in one rebuild, rather than one call each " +
			"rebuilding the scenario and cancelling the warm the last one started",
		Params: []state.Param{
			{Name: "nodes", Type: state.ParamArray, Primary: true,
				What: "the names to remove; one this network has not got refuses " +
					"the whole call and removes nothing, and naming none is " +
					"accepted and does nothing"},
		},
		Returns: []string{"deleted", "nodes"},
		Answers: "`deleted` is the names that went and `nodes` is how many are " +
			"left. Every link is dropped and the matrix re-measured behind the " +
			"answer, because the network is not the one that was measured.",
		Example: &state.Example{
			Params: map[string]any{"nodes": []any{"Dunfermline"}},
			What:   "take one node out of the network",
		},
	}, func(w *state.World, p any) (any, error) {
		want, err := nameSet(p, "nodes")
		if err != nil {
			return nil, err
		}
		return s.dropNodes(st, w, want, "deleted")
	})

	// The complement rather than the set, because that is how somebody
	// actually says it - "just these two" - and because working it out on the
	// client means fetching the list first and racing whatever else is
	// changing it.
	st.HandleSpec("nodes.keep", state.Spec{
		What: "cut a network down to the nodes named and remove everything else, " +
			"which is how trimming a fixture is actually said",
		Params: []state.Param{
			{Name: "nodes", Type: state.ParamArray, Primary: true,
				What: "the names to keep; one this network has not got refuses " +
					"the whole call and removes nothing, and naming none keeps " +
					"nothing, which empties the network"},
		},
		Returns: []string{"deleted", "nodes"},
		Answers: "`deleted` names what was removed rather than what was kept, " +
			"and `nodes` is how many are left.",
		Example: &state.Example{
			Params: map[string]any{"nodes": []any{"West Lomond"}},
			What:   "cut a network down to one node",
		},
	}, func(w *state.World, p any) (any, error) {
		keep, err := nameSet(p, "nodes")
		if err != nil {
			return nil, err
		}
		for name := range keep {
			if !s.hasNode(name) {
				return nil, noSuchNode(name)
			}
		}
		drop := map[string]bool{}
		for _, n := range s.nodes {
			if !keep[n.Name] {
				drop[n.Name] = true
			}
		}
		return s.dropNodes(st, w, drop, "deleted")
	})

	// It rebuilds and re-warms for the same reason moving a node does.
	st.HandleSpec("node.set_board", state.Spec{
		What: "say what hardware a node is, which is a change to the physics and " +
			"not a label: the transmit ceiling, the receive chain's noise figure " +
			"and the battery the energy model runs on all come off the board",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true, Primary: true,
				What: "which node changes hardware; absent or unknown is refused"},
			{Name: "board", Type: state.ParamString,
				What: "the board it is, as the firmware library names one; a " +
					"board this build has no profile for is refused, and an " +
					"empty or absent name returns the node to no particular " +
					"hardware, which is a build for this machine"},
		},
		Returns: []string{"node", "board"},
		Answers: "A build pinned for the old board is cleared rather than carried " +
			"across, because that image is for that hardware and a node keeping " +
			"the pin would look configured and refuse at start.",
		Example: &state.Example{
			Params: map[string]any{"node": "West Lomond", "board": "Heltec_WSL3"},
			What:   "make a node a Heltec WSL3",
		},
	}, func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "node")
		if name == "" {
			return nil, control.WithCode(control.BadParams,
				fmt.Errorf("node.set_board needs a node"))
		}
		want, _ := namedField(p, "board")
		resolved := ""
		if want != "" {
			board, err := hw.BoardByName(want)
			if err != nil {
				return nil, control.WithCode(control.BadParams, err)
			}
			resolved = board.Name
		}
		nodes := snapshotNodes(s.nodes)
		found := false
		for i := range nodes {
			if nodes[i].Name != name {
				continue
			}
			found = true
			nodes[i].Board = resolved
			// The build follows the hardware, or stops claiming to.
			//
			// A build pinned for one board cannot run on another - it is that
			// image for that hardware - so carrying the old pin across would
			// leave a node that looks configured and refuses at start. Cleared
			// instead, which reads as "choose one" rather than as a lie.
			if nodes[i].Firmware.Board != "" && nodes[i].Firmware.Board != resolved {
				nodes[i].Firmware.Board = ""
				nodes[i].Firmware.Version = ""
			}
		}
		if !found {
			return nil, noSuchNode(name)
		}
		s.buildSeeded(nodes, s.freqMHz, s.seed)
		w.Nodes = stateNodes(nodes)
		w.Links = nil
		s.warm(st, len(nodes))
		if resolved == "" {
			w.Say(name + " is no longer any particular board")
		} else {
			w.Say(name + " is a " + resolved)
		}
		return map[string]any{"node": name, "board": resolved}, nil
	})
}

// dropNodes removes a set and rebuilds once, whatever its size.
func (s *Sim) dropNodes(st *state.Store, w *state.World,
	drop map[string]bool, said string) (any, error) {
	if len(drop) == 0 {
		// Not an error. "keep everything" and "delete nothing" are both
		// coherent requests, and refusing them would make a script guard a
		// call it should be able to make unconditionally.
		return map[string]any{said: []string{}, "nodes": len(s.nodes)}, nil
	}
	var missing []string
	for name := range drop {
		if !s.hasNode(name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		// Named, all of them, and nothing removed.
		//
		// Half a deletion is worse than none: a script that asked for six and
		// got four has a scenario nobody described, and no way to tell which
		// two it still has without asking again.
		return nil, control.WithCode(control.NotFound, fmt.Errorf(
			"no node named %s", strings.Join(quoted(missing), ", ")))
	}
	kept := make([]scenario.Node, 0, len(s.nodes))
	var gone []string
	for _, n := range s.nodes {
		if drop[n.Name] {
			gone = append(gone, n.Name)
			continue
		}
		kept = append(kept, n)
	}
	s.buildSeeded(kept, s.freqMHz, s.seed)
	out := w.Nodes[:0]
	for _, n := range w.Nodes {
		if !drop[n.Name] {
			out = append(out, n)
		}
	}
	w.Nodes = out
	w.Links = nil
	s.warm(st, len(kept))
	w.Say(fmt.Sprintf("deleted %d: %s", len(gone), strings.Join(gone, ", ")))
	return map[string]any{said: gone, "nodes": len(kept)}, nil
}

// hasNode reports whether the scenario holds one by that name.
func (s *Sim) hasNode(name string) bool {
	for _, n := range s.nodes {
		if n.Name == name {
			return true
		}
	}
	return false
}

// nameSet reads a list of node names, however it was written.
//
// A bare list, or an object with one - because the socket unwraps a JSON array
// to []string and a caller writing an object is equally reasonable, and a verb
// that accepted only one of those would be a verb people got wrong once each.
func nameSet(p any, key string) (map[string]bool, error) {
	var names []string
	switch v := p.(type) {
	case []string:
		names = v
	case string:
		names = []string{v}
	case map[string]any:
		switch inner := v[key].(type) {
		case []any:
			for _, x := range inner {
				s, ok := x.(string)
				if !ok {
					return nil, control.WithCode(control.BadParams,
						fmt.Errorf("%s must be a list of node names", key))
				}
				names = append(names, s)
			}
		case []string:
			names = inner
		case string:
			names = []string{inner}
		}
	}
	out := map[string]bool{}
	for _, n := range names {
		if n != "" {
			out[n] = true
		}
	}
	return out, nil
}

func quoted(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, fmt.Sprintf("%q", s))
	}
	return out
}
