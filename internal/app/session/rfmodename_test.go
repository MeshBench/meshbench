package session

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// A sweep builds an engine per cell and had no way to ask which physics the
// session was switched to, so every cell ran the zero value - calculated -
// however the workbench was set, and said nothing about it.
func TestTheSessionSaysWhichPhysicsItUses(t *testing.T) {
	var s Sim
	if got := s.RFModeName(); got != "calculated" {
		t.Errorf("an untouched session reports %q, want calculated", got)
	}
	if got := RFModeFor(s.RFModeName()); got != engine.RFCalculated {
		t.Errorf("calculated resolved to %v", got)
	}

	s.rfMode = "waveform"
	if got := s.RFModeName(); got != "waveform" {
		t.Errorf("a session switched to waveform reports %q", got)
	}
	if got := RFModeFor(s.RFModeName()); got != engine.RFWaveform {
		t.Errorf("waveform resolved to %v, want the waveform model", got)
	}
}
