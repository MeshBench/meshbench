package session

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A node placed into a running mesh runs what the mesh runs; a mesh with
// nothing to copy leaves the ref empty for sim.start's message to explain.
func TestFirmwareOfNeighboursJoinsTheMesh(t *testing.T) {
	ref := func(v string) scenario.FirmwareRef {
		return scenario.FirmwareRef{Role: "repeater", Version: v}
	}
	nodes := []scenario.Node{
		{Kind: scenario.SimpleRepeater, Firmware: ref("v1.17.1")},
		{Kind: scenario.SimpleRepeater, Firmware: ref("v1.17.1")},
		{Kind: scenario.AdvancedRepeater, Firmware: ref("v1.16.0")},
		{Kind: scenario.Companion, Firmware: scenario.FirmwareRef{Role: "companion_radio", Version: "v9"}},
	}
	got := firmwareOfNeighbours(nodes, scenario.SimpleRepeater)
	if got.Version != "v1.17.1" || got.Role != "repeater" {
		t.Fatalf("joined with %+v, want the mesh's v1.17.1 repeater build", got)
	}
	if got := firmwareOfNeighbours(nodes, scenario.SDRObserver); got.Version != "" {
		t.Fatalf("an observer got firmware %+v; it runs none", got)
	}
	if got := firmwareOfNeighbours(nil, scenario.SimpleRepeater); got.Version != "" {
		t.Fatalf("an empty mesh invented firmware %+v", got)
	}
}
