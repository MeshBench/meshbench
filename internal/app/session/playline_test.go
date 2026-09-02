package session

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Playing over a matrix nobody measured has to say so.
//
// No links means every transmission reaches nobody while the nodes, the counts
// and the console all report success. That was reported as an advert no node
// receives, and the only sentence explaining it was the one playing used to
// overwrite. The cause then was a warm held for terrain consent; nothing holds
// a warm any more, but a warm that failed or was cancelled leaves the same
// empty matrix and the operator needs the same sentence.
func TestPlayingWithNoMatrixSaysNothingCanBeReceived(t *testing.T) {
	_, s := register(t)

	// An empty session says nothing, because there is nothing to measure
	// between and no traffic anybody is about to send.
	if got := s.playLine(); got != "playing" {
		t.Fatalf("an empty session was warned at: %q", got)
	}

	s.nodes = []scenario.Node{
		{Name: "a", Position: scenario.LatLon{Lat: 56.0, Lon: -4.0}},
		{Name: "b", Position: scenario.LatLon{Lat: 56.2, Lon: -4.3}},
	}

	// Nodes, and nothing measured or measuring: the case worth a sentence.
	got := s.playLine()
	if got == "playing" {
		t.Fatal("a run over an empty matrix said only \"playing\"; the operator " +
			"is about to send traffic that reaches nobody")
	}
	for _, want := range []string{"no link has been measured", "Warm the links again"} {
		if !strings.Contains(got, want) {
			t.Errorf("the empty-matrix line does not carry %q: %s", want, got)
		}
	}

	// A measured matrix is not nagged at.
	s.warmMu.Lock()
	s.warmed = true
	s.warmMu.Unlock()
	if got := s.playLine(); got != "playing" {
		t.Errorf("a measured run was warned at anyway: %s", got)
	}
}
