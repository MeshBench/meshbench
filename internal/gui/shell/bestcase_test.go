package shell

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/gui/state"
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
