package shell

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// The caveat line must tell the truth about the switches: "ideal
// demodulator" over a waveform run is a wrong caveat, and a loaded
// environment is not bare earth.
func TestBestCaseLineFollowsTheSwitches(t *testing.T) {
	kind := bestCaseLine(&state.Snapshot{})
	for _, want := range []string{"no multipath", "bare earth", "ideal demodulator"} {
		if !strings.Contains(kind, want) {
			t.Fatalf("kind defaults lost %q: %s", want, kind)
		}
	}
	wf := bestCaseLine(&state.Snapshot{RFMode: "waveform",
		RFRealism: state.RFRealism{MultipathDB: 12}, RFEnvironment: "/tiles"})
	for _, gone := range []string{"ideal demodulator", "no multipath", "bare earth"} {
		if strings.Contains(wf, gone) {
			t.Fatalf("honest run still carries %q: %s", gone, wf)
		}
	}
	if !strings.Contains(wf, "real coding chain") {
		t.Fatalf("waveform run does not say what decides it: %s", wf)
	}
	if bestCaseLine(nil) == "" {
		t.Fatal("a nil snapshot must still produce the kind-default line")
	}
}

// The missing-terrain caveat leads, and only appears when it is true. It is
// the one clause here that can become true without anybody having chosen it,
// and free space is a bigger claim than the three beside it.
func TestBestCaseLineLeadsWithMissingTerrain(t *testing.T) {
	bare := bestCaseLine(&state.Snapshot{Ground: state.Ground{State: state.GroundBare}})
	if !strings.Contains(bare, "NO TERRAIN") {
		t.Fatalf("a study with no ground under it does not say so: %s", bare)
	}
	if i := strings.Index(bare, "NO TERRAIN"); i > strings.Index(bare, "no multipath") {
		t.Errorf("the biggest caveat is not first: %s", bare)
	}
	full := bestCaseLine(&state.Snapshot{
		Ground: state.Ground{State: state.GroundTerrain, Sampled: 40, Cached: 40}})
	if strings.Contains(full, "TERRAIN") {
		t.Errorf("a study standing on real ground still claims it is not: %s", full)
	}
	// And a session nothing has looked at yet must not claim either way.
	if strings.Contains(bestCaseLine(&state.Snapshot{}), "TERRAIN") {
		t.Error("an unexamined session claims its ground is missing")
	}
}
