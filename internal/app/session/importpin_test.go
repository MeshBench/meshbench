package session

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// An import carries no firmware version - a deployment's feed says what a node
// is and where it stands, not which MeshCore it should be simulated with - and
// nothing filled the gap. So a committed import left every node with an empty
// version, started them anyway through the override path where the version is
// never consulted, counted all of them as running, and produced no traffic at
// all: 562 nodes, two simulated minutes, zero events.
func TestAnImportPinsTheNewestBuildForEachRole(t *testing.T) {
	latest := map[string]string{
		"simple_repeater": "v1.17.1",
		"companion_radio": "v1.17.1",
	}
	nodes := []scenario.Node{
		{Name: "a repeater", Kind: scenario.SimpleRepeater},
		{Name: "a companion", Kind: scenario.Companion},
		// An observer runs no firmware and must not be given one.
		{Name: "an observer", Kind: scenario.SDRObserver},
		// Already pinned by hand: left alone, so an "add" onto a scenario
		// somebody has set up does not overwrite their choice.
		{Name: "pinned", Kind: scenario.SimpleRepeater,
			Firmware: scenario.FirmwareRef{Role: "simple_repeater", Version: "v1.16.0"}},
	}

	if n := pinFrom(nodes, latest); n != 2 {
		t.Errorf("pinned %d nodes, want the two that needed one", n)
	}
	if got := nodes[0].Firmware.Version; got != "v1.17.1" {
		t.Errorf("the repeater is pinned to %q", got)
	}
	if got := nodes[0].Firmware.Role; got != scenario.RoleSimpleRepeater {
		t.Errorf("the repeater's role is %q", got)
	}
	if got := nodes[1].Firmware.Role; got != scenario.RoleCompanionRadio {
		t.Errorf("the companion's role is %q", got)
	}
	if got := nodes[2].Firmware.Version; got != "" {
		t.Errorf("an observer runs no firmware and was pinned to %q", got)
	}
	if got := nodes[3].Firmware.Version; got != "v1.16.0" {
		t.Errorf("a node pinned by hand was overwritten with %q", got)
	}
}

// Nothing on the disk, nothing pinned - and silently, because the start gate
// is the place that says "no firmware for N of M nodes" and names them.
func TestAnImportWithNoBuildsPinsNothing(t *testing.T) {
	nodes := []scenario.Node{{Name: "a repeater", Kind: scenario.SimpleRepeater}}
	if n := pinFrom(nodes, nil); n != 0 {
		t.Errorf("pinned %d nodes with an empty cache", n)
	}
	if nodes[0].Firmware.Version != "" {
		t.Errorf("a node was pinned to %q from nothing", nodes[0].Firmware.Version)
	}
}
