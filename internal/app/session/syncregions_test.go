package session

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// The snapshot's region columns are re-projected from the scenario every
// readout, so a region applied while the mesh runs cannot vanish from
// nodes.list even if whatever rebuilds the rows drops it (#737). The scenario
// is authoritative: it is what the firmware was provisioned from.
func TestSyncRegionsMakesTheSnapshotFollowTheScenario(t *testing.T) {
	s := &Sim{nodes: []scenario.Node{
		{Name: "A", Regions: []string{"fif"}, DefaultScope: "fif"},
		{Name: "B"},
	}}
	// The snapshot has drifted: A holds the regions in the scenario but the
	// row does not, which is the state the bug leaves.
	w := &state.World{Nodes: []state.Node{{Name: "A"}, {Name: "B"}}}

	s.syncRegions(w)

	if got := w.Nodes[0].Regions; len(got) != 1 || got[0] != "fif" {
		t.Errorf("A's row has regions %v, want [fif] from the scenario", got)
	}
	if w.Nodes[0].DefaultScope != "fif" {
		t.Errorf("A's row default scope is %q, want fif", w.Nodes[0].DefaultScope)
	}
	if len(w.Nodes[1].Regions) != 0 {
		t.Errorf("B holds no region in the scenario, so its row should hold none: %v", w.Nodes[1].Regions)
	}

	// A row with no scenario node of that name is left alone rather than
	// cleared: the scenario decides what exists.
	w2 := &state.World{Nodes: []state.Node{{Name: "ghost", Regions: []string{"x"}}}}
	s.syncRegions(w2)
	if len(w2.Nodes[0].Regions) != 1 {
		t.Error("a row with no matching scenario node was cleared")
	}
}
