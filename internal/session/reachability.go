// Reachability, in both directions.
//
// CLAUDE.md is explicit that a result which does not say which direction
// works is wrong even when the arithmetic is right, and a fault's before-
// and-after counts are exactly the kind of result that can quietly forget
// that. reachCounts always returns the pair.
package session

import "github.com/MeshBench/meshbench/internal/gui/state"

// reachCounts is how many other nodes from index can flood-reach (out) and
// be flood-reached by (in), walking w.Links as a directed graph - an edge
// a→b exists when the link's margin in that direction closes, and down
// removes a node from the graph entirely: it neither relays through nor
// answers at a node that is not running.
//
// A pure function of the link table and the down set, not of the live
// engine, so a fault's reachability is testable against a hand-built
// network without any firmware attached.
func reachCounts(links []state.Link, from int, down map[int]bool) (out, in int) {
	if down[from] {
		return 0, 0
	}
	return bfsCount(links, from, down, true), bfsCount(links, from, down, false)
}

func bfsCount(links []state.Link, from int, down map[int]bool, forward bool) int {
	seen := map[int]bool{from: true}
	queue := []int{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, l := range links {
			if !l.Known {
				continue
			}
			var next int
			var margin float64
			switch cur {
			case l.A:
				next = l.B
				if forward {
					margin = l.AtoB
				} else {
					margin = l.BtoA
				}
			case l.B:
				next = l.A
				if forward {
					margin = l.BtoA
				} else {
					margin = l.AtoB
				}
			default:
				continue
			}
			if margin <= 0 || down[next] || seen[next] {
				continue
			}
			seen[next] = true
			queue = append(queue, next)
		}
	}
	return len(seen) - 1 // excludes from itself
}
