package session

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/provision"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// A build with no firmware attached - the common case in a test, and the
// state a real network is in for the few seconds before AttachNativeProgress
// finishes - must return an empty context rather than erroring or panicking.
func TestProvisioningContextIsEmptyWithNoFirmwareAttached(t *testing.T) {
	st, _ := newBoundaryTestSim(t, []scenario.Node{repeaterNode("Solo")})
	res, err := st.Do(context.Background(), "provisioning.context", nil)
	if err != nil {
		t.Fatal(err)
	}
	pc, ok := res.(provisioningContext)
	if !ok {
		t.Fatalf("got %T", res)
	}
	if len(pc.Nodes) != 0 {
		t.Errorf("no firmware is attached in this build, wanted zero nodes, got %d", len(pc.Nodes))
	}
}

func TestProvisioningStoreReadbackRejectsTheWrongType(t *testing.T) {
	st, _ := newBoundaryTestSim(t, nil)
	if _, err := st.Do(context.Background(), "provisioning.store-readback", "not a readback"); err == nil {
		t.Fatal("wanted a refusal for the wrong param type, got none")
	}
}

func TestProvisioningStoreResultsRejectsTheWrongType(t *testing.T) {
	st, _ := newBoundaryTestSim(t, nil)
	if _, err := st.Do(context.Background(), "provisioning.store-results", 42); err == nil {
		t.Fatal("wanted a refusal for the wrong param type, got none")
	}
}

// Once a readback is stored, refreshMatches has to actually change the
// snapshot's match counts - the whole point of a rule showing "0 nodes
// match" while it is being written.
func TestStoringAReadbackUpdatesMatchCounts(t *testing.T) {
	st, s := newBoundaryTestSim(t, nil)
	s.rules = []provision.Rule{
		{Name: "repeaters", Conditions: []provision.Condition{
			{Field: "kind", Op: "is", Value: "simple-repeater"},
		}},
	}
	readback := map[string]provision.NodeState{
		"A": {Read: true, Kind: "simple-repeater"},
		"B": {Read: true, Kind: "companion"},
	}
	if _, err := st.Do(context.Background(), "provisioning.store-readback", readback); err != nil {
		t.Fatal(err)
	}
	snap := st.Snapshot()
	if !snap.ProvisioningRead {
		t.Error("ProvisioningRead should be true once a readback has been stored")
	}
	if got := snap.ProvisioningMatch["repeaters"]; got != 1 {
		t.Errorf("got %d matches, wanted 1 (only node A is a simple-repeater)", got)
	}
}
