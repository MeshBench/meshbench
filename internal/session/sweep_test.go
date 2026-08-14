package session

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/scenario"
)

func nodes() []scenario.Node {
	return []scenario.Node{
		{Name: "rep", Kind: scenario.SimpleRepeater},
		{Name: "comp", Kind: scenario.Companion},
		{Name: "obs", Kind: scenario.SDRObserver},
	}
}

// An arm pins by role, and pins both roles. A sweep that only set the repeater
// left companions on whatever the scenario carried, which for a fresh import is
// nothing - and the arm then measured a mesh that could not talk.
func TestAnArmPinsBothRoles(t *testing.T) {
	arm := SweepArm{
		Name: "1.17", RepeaterVersion: "repeater-v1.17.0",
		CompanionVersion: "companion-v1.17.0",
	}
	got := map[string]string{}
	for _, n := range nodes() {
		got[n.Name] = withFirmware(n, arm).Firmware.Version
	}
	if got["rep"] != "repeater-v1.17.0" {
		t.Errorf("repeater got %q", got["rep"])
	}
	if got["comp"] != "companion-v1.17.0" {
		t.Errorf("companion got %q", got["comp"])
	}
	// An observer runs no firmware, and pinning one would have the engine
	// looking for a build that the role does not publish.
	if got["obs"] != "" {
		t.Errorf("observer was pinned to %q", got["obs"])
	}
}

// The scenario is shared by every cell, so an arm must not write to it. This is
// the silent failure the firmware A/B tool was written to prevent: arm two
// inherits arm one's version and both report the same numbers while looking
// fine.
func TestAnArmDoesNotEditTheScenario(t *testing.T) {
	original := nodes()
	arm := SweepArm{RepeaterVersion: "repeater-v1.16.0", CompanionVersion: "companion-v1.16.0"}
	for _, n := range original {
		_ = withFirmware(n, arm)
	}
	for _, n := range original {
		if n.Firmware.Version != "" {
			t.Fatalf("%s was left pinned to %q after an arm ran", n.Name, n.Firmware.Version)
		}
	}
}

// An empty arm leaves the scenario's own pin alone, so a sweep about something
// other than firmware does not silently unpin every node.
func TestAnEmptyArmChangesNothing(t *testing.T) {
	n := scenario.Node{Name: "rep", Kind: scenario.SimpleRepeater}
	n.Firmware.Version = "repeater-v1.17.0"
	if got := withFirmware(n, SweepArm{Name: "load only"}).Firmware.Version; got != "repeater-v1.17.0" {
		t.Fatalf("an arm with no versions changed the pin to %q", got)
	}
}
