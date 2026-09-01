package meshbench

import (
	"errors"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/control"
)

// The pairing rule, from this end: a client and the workbench it drives must be
// the same release.

func TestThePairingRuleIsExactMatchOrAnUnstampedEnd(t *testing.T) {
	cases := []struct {
		ours, theirs string
		want         bool
	}{
		{"1.0.0", "1.0.0", true},
		{"1.0.0", "1.0.1", false},
		{"2.0.0", "1.0.0", false},
		{"", "1.0.0", true},
		{"1.0.0", "", true},
		{"", "", true},
	}
	for _, c := range cases {
		if got := pairedRelease(c.ours, c.theirs); got != c.want {
			t.Errorf("a %q client against a %q workbench is served=%v, want %v",
				c.ours, c.theirs, got, c.want)
		}
	}
}

// A check that compared nothing has to say so, or a pair nothing verified looks
// exactly like a pair that was checked and matched.
func TestASkippedCheckSaysWhyItWasSkipped(t *testing.T) {
	if note := pairingNote("1.0.0", "1.0.0"); note != "" {
		t.Errorf("a check that did compare says %q, want nothing", note)
	}
	for _, c := range []struct{ ours, theirs, want string }{
		{"", "1.0.0", "this client is a development build"},
		{"1.0.0", "", "the workbench is a development build"},
		{"", "", "neither"},
	} {
		note := pairingNote(c.ours, c.theirs)
		if !strings.Contains(note, "skipped") || !strings.Contains(note, c.want) {
			t.Errorf("a %q client against a %q workbench says %q, want it to "+
				"mention %q", c.ours, c.theirs, note, c.want)
		}
	}
}

// The workbench refuses the pair itself, on the frame this client declared it
// on. That refusal has to reach the caller as the mismatch it is, not as
// session.hello failing, which is the confusion the declaration exists to end.
func TestTheWorkbenchsOwnRefusalArrivesAsAVersionMismatch(t *testing.T) {
	said := "this client is from MeshBench 1.5.0 and this workbench is " +
		"MeshBench 2.0.0. A client and the workbench it drives must be the " +
		"same release: install the 2.0.0 client, or run the 1.5.0 workbench"
	err := asMismatch(wrap("session.hello", &control.Coded{
		Code: control.VersionMismatch, Err: errors.New(said),
	}))

	var mismatch *VersionMismatch
	if !errors.As(err, &mismatch) {
		t.Fatalf("a release refusal came back as %T: %v", err, err)
	}
	// The workbench's own words, kept whole: it knows both releases, and a
	// paraphrase would lose the half this client cannot see.
	if mismatch.Error() != said {
		t.Errorf("the workbench's refusal was rewritten:\n got %s\nwant %s",
			mismatch.Error(), said)
	}
	// And it is not the other mismatch, because the remedies differ.
	var wrongKind *ProtocolMismatch
	if errors.As(err, &wrongKind) {
		t.Error("a release refusal is also reported as a protocol mismatch")
	}
}

// The mismatch this client notices itself - against a workbench old enough to
// ignore what the client declared - names both releases and what to do.
func TestAMismatchThisClientNoticedNamesBothReleases(t *testing.T) {
	e := &VersionMismatch{Client: "1.5.0", Workbench: "2.0.0"}
	for _, want := range []string{"1.5.0", "2.0.0", "must be the same release"} {
		if !strings.Contains(e.Error(), want) {
			t.Errorf("the mismatch does not say %q: %s", want, e.Error())
		}
	}
}
