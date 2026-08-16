package session

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/scenario"
)

// warm hands its worker the network it was started for, and that hand-off has
// to be a copy of the nodes rather than of the slice header.
//
// Assigning s.nodes shares the backing array, so the worker reading a node's
// fields and a verb writing them on the store goroutine touch the same memory.
// The race detector caught exactly that in CI - warmOnGPU reading a node while
// setFirmware wrote its firmware version - and it is invisible on a developer
// machine, where the two rarely overlap.
func TestSnapshotNodesIsACopyNotAnAlias(t *testing.T) {
	live := []scenario.Node{
		{Name: "Alpha"}, {Name: "Beta"},
	}
	live[0].Firmware.Version = "repeater-v1.17.0"

	taken := snapshotNodes(live)

	// What the store goroutine does while a warm is in flight.
	live[0].Firmware.Version = "repeater-v1.16.0"
	live[1].Name = "renamed"

	if taken[0].Firmware.Version != "repeater-v1.17.0" {
		t.Errorf("the snapshot saw a later write: version = %q, want the one taken at the time",
			taken[0].Firmware.Version)
	}
	if taken[1].Name != "Beta" {
		t.Errorf("the snapshot saw a later write: name = %q, want Beta", taken[1].Name)
	}
	if len(taken) != len(live) {
		t.Errorf("snapshot has %d nodes, want %d", len(taken), len(live))
	}
}

// An empty network is a network, and a warm started against one must not be a
// special case at the call site.
func TestSnapshotNodesHandlesNothing(t *testing.T) {
	if got := snapshotNodes(nil); len(got) != 0 {
		t.Errorf("snapshotNodes(nil) = %v, want empty", got)
	}
}
