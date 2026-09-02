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
	st.Handle("nodes.delete_many", func(w *state.World, p any) (any, error) {
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
	st.Handle("nodes.keep", func(w *state.World, p any) (any, error) {
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
	st.Handle("node.set_board", func(w *state.World, p any) (any, error) {
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
//
// A shape outside that set is refused rather than read as no names. Naming none
// is a documented answer for both verbs that use this, and for nodes.keep that
// answer is "empty the network": a parameter nothing recognised - an object
// keyed `names`, as the selection verbs key theirs, or a bare number - fell
// through to it and deleted every node while reporting success.
func nameSet(p any, key string) (map[string]bool, error) {
	names, err := nodeNamesIn(p, key)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, n := range names {
		if n != "" {
			out[n] = true
		}
	}
	return out, nil
}

// nodeNamesIn is nameSet's reader, kept apart so the switch stays one switch.
func nodeNamesIn(p any, key string) ([]string, error) {
	switch v := p.(type) {
	case nil:
		return nil, nil
	case []string:
		return v, nil
	case string:
		return []string{v}, nil
	case map[string]any:
		return namesUnder(v, key)
	}
	return nil, control.WithCode(control.BadParams, fmt.Errorf(
		"node names come as a list, one name, or {%q: [...]}; this was a %T", key, p))
}

// namesUnder is the object form.
func namesUnder(m map[string]any, key string) ([]string, error) {
	raw, present := m[key]
	if !present {
		return nil, control.WithCode(control.BadParams, fmt.Errorf(
			"an object with no %q in it; node names come as a list, one name, "+
				"or {%q: [...]}", key, key))
	}
	switch inner := raw.(type) {
	case nil:
		return nil, nil
	case []string:
		return inner, nil
	case string:
		return []string{inner}, nil
	case []any:
		out := make([]string, 0, len(inner))
		for _, x := range inner {
			s, ok := x.(string)
			if !ok {
				return nil, control.WithCode(control.BadParams,
					fmt.Errorf("%s must be a list of node names", key))
			}
			out = append(out, s)
		}
		return out, nil
	}
	return nil, control.WithCode(control.BadParams,
		fmt.Errorf("%s is a %T; it takes a list of node names", key, raw))
}

func quoted(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, fmt.Sprintf("%q", s))
	}
	return out
}
