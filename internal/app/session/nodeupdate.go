// One walk over the nodes, writing what runs and what is drawn together.
package session

import (
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// updateNodes visits every node of the scenario once, handing the change both
// the scenario node and the snapshot row that names it, and returns how many
// the change reported it made.
//
// One walk rather than two, because two walks over the same nodes have to
// agree about which nodes they are and twice they did not. A role filter was
// honoured while writing the scenario and dropped while writing the snapshot,
// so pinning a build to the repeaters redrew every node in the mesh as running
// it. Regions were matched on the public key a packet actually carries on one
// side and on the display name on the other, so the verb reported forty-four
// nodes and the map coloured four. Both faults preserved the count and got the
// set wrong, which is why a count is the last thing to compare them by.
//
// The row is nil where the snapshot holds no node of that name: it is the
// scenario that decides what exists, and the view is what has to keep up.
func (s *Sim) updateNodes(w *state.World,
	change func(n *scenario.Node, row *state.Node) bool) int {

	rows := make(map[string]int, len(w.Nodes))
	for i := range w.Nodes {
		rows[w.Nodes[i].Name] = i
	}
	changed := 0
	for i := range s.nodes {
		var row *state.Node
		if j, ok := rows[s.nodes[i].Name]; ok {
			row = &w.Nodes[j]
		}
		if change(&s.nodes[i], row) {
			changed++
		}
	}
	return changed
}
