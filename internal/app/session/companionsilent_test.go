package session

import (
	"strings"
	"testing"
)

// "It says advert sent and nothing transmits."
//
// Writing at a serial port succeeds whether or not anything is reading it, so
// every companion command reported itself sent - against a board whose
// firmware never started, against a build that is not a companion at all,
// against a node that had crashed twenty minutes ago. The interface was the
// last thing anybody would suspect, because it was telling the truth about
// what it had done and nothing about what had happened.
//
// Measured: an imported build that does not run answered "advert sent" with
// zero transmissions on the air, while the stock build put one up and had it
// heard.
func TestACompanionThatHasNeverAnsweredIsNotSentTo(t *testing.T) {
	c := &compSession{node: "Comp"}
	err := silentCompanion(c, "Comp")
	if err == nil {
		t.Fatal("a node that has said nothing at all was treated as listening")
	}
	// The refusal has to say where to look, or it is a wall.
	for _, want := range []string{"Comp", "not answered", "Output tab"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q: %v", want, err)
		}
	}

	// One decoded frame is a node that is talking, and it stays trusted: a
	// node can be busy without being dead, and refusing on a slow reply would
	// be worse than the fault this catches.
	c.rev = 1
	if err := silentCompanion(c, "Comp"); err != nil {
		t.Errorf("a node that has answered was refused: %v", err)
	}
}
