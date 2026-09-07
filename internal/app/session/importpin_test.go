package session

import (
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"

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

// Newest by when it was built, not by the version string: repeater-v1.17.1
// sorts before repeater-v1.9.0 as a string, and a local build sorts before
// every tag however new it is.
func TestTheNewestBuildIsByTimeNotByString(t *testing.T) {
	built := map[string]time.Time{
		"repeater-v1.9.0":    time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		"repeater-v1.17.1":   time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		"local-this-morning": time.Date(2026, 9, 7, 9, 0, 0, 0, time.UTC),
		"companion-v1.17.1":  time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
	}
	installed := []firmware.Installed{
		{Native: true, Role: "simple_repeater", Version: "repeater-v1.17.1"},
		{Native: true, Role: "simple_repeater", Version: "repeater-v1.9.0"},
		{Native: true, Role: "companion_radio", Version: "companion-v1.17.1"},
		// A board image never counts, however new.
		{Native: false, Role: "simple_repeater", Version: "v9.9.9", Board: "Heltec_v3"},
	}
	at := func(b firmware.Installed) time.Time { return built[b.Version] }

	got := newestByRole(installed, at)
	if got["simple_repeater"] != "repeater-v1.17.1" {
		t.Errorf("with v1.9 and v1.17 installed the newest repeater is %q", got["simple_repeater"])
	}
	if got["companion_radio"] != "companion-v1.17.1" {
		t.Errorf("the newest companion is %q", got["companion_radio"])
	}

	// A local build made this morning is the newest build on this machine.
	installed = append(installed, firmware.Installed{
		Native: true, Role: "simple_repeater", Version: "local-this-morning"})
	if got := newestByRole(installed, at)["simple_repeater"]; got != "local-this-morning" {
		t.Errorf("a build made this morning lost to %q", got)
	}
}
