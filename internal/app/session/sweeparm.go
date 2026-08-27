package session

import "github.com/MeshBench/meshbench/internal/world/scenario"

// SweepArm is one configuration under test - shared by the sweep verbs and the
// experiment matrix, so it lives in core rather than in either domain package.
type SweepArm struct {
	Name string
	// EveryMs is how often the originator sends. The parameter being swept
	// here is offered load, which is the one the map already showed buys
	// redundancy rather than delivery.
	EveryMs uint32
	// RepeaterVersion and CompanionVersion put a firmware build under test.
	//
	// This is what lets a sweep answer "does this release behave differently",
	// which is the question the listen-before-talk study asked and which an
	// offered-load sweep cannot reach. Empty leaves the scenario's own pin
	// alone.
	RepeaterVersion  string
	CompanionVersion string
}

// WithFirmware returns the node with this arm's build pinned, by role.
func WithFirmware(n scenario.Node, arm SweepArm) scenario.Node {
	switch {
	case n.Kind == scenario.Companion && arm.CompanionVersion != "":
		n.Firmware.Version = arm.CompanionVersion
		n.Firmware.Role = "companion_radio"
	case n.Kind.RunsFirmware() && n.Kind != scenario.Companion && arm.RepeaterVersion != "":
		n.Firmware.Version = arm.RepeaterVersion
		n.Firmware.Role = "simple_repeater"
	}
	return n
}
