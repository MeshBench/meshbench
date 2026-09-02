package session

import (
	"strings"
	"testing"
)

// Playing over a matrix nobody measured has to say so.
//
// A warm held for terrain consent leaves no links, and no links means every
// transmission reaches nobody while the nodes, the counts and the console all
// report success. That was reported as an advert no node receives, and the
// only sentence explaining it was the one playing used to overwrite.
func TestPlayingOverAHeldWarmSaysNothingCanBeReceived(t *testing.T) {
	_, s := register(t)

	// The ordinary case first, so the interesting one reads as a difference
	// rather than as an assumption about what this says.
	if got := s.playLine(); got != "playing" {
		t.Fatalf("with nothing held, playing says %q; want the plain word", got)
	}

	s.warmMu.Lock()
	s.warmed, s.warmHeld = false, true
	s.warmMu.Unlock()

	got := s.playLine()
	if got == "playing" {
		t.Fatal("a run over a held warm said only \"playing\"; the operator is " +
			"about to send traffic that reaches nobody")
	}
	for _, want := range []string{"no link has been measured", "terrain.allow"} {
		if !strings.Contains(got, want) {
			t.Errorf("the held-warm line does not carry %q: %s", want, got)
		}
	}

	// And a measured matrix is not nagged at.
	s.warmMu.Lock()
	s.warmed, s.warmHeld = true, false
	s.warmMu.Unlock()
	if got := s.playLine(); got != "playing" {
		t.Errorf("a measured run was warned at anyway: %s", got)
	}
}
