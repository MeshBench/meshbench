package meshbench

import (
	"errors"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/control"
)

// A workbench that refuses this client's wire version must reach the caller as
// a mismatch it can branch on, not as session.hello failing - which is the
// confusion the whole declaration exists to end.
func TestAVersionRefusalArrivesAsAMismatch(t *testing.T) {
	said := "this client speaks control protocol 1 and this workbench " +
		"(v9.9.9) speaks 2. Upgrade this client to the one that ships with v9.9.9"
	err := asMismatch(wrap("session.hello", &control.Coded{
		Code: control.ProtocolMismatch, Err: errors.New(said),
	}))

	var mismatch *ProtocolMismatch
	if !errors.As(err, &mismatch) {
		t.Fatalf("a version refusal came back as %T: %v", err, err)
	}
	if mismatch.Client != control.Protocol {
		t.Errorf("it reports this client speaking %d, want %d",
			mismatch.Client, control.Protocol)
	}
	// The workbench's own words, kept whole: it knows its build and which end
	// is the older one, and a paraphrase would lose both.
	if mismatch.Error() != said {
		t.Errorf("the workbench's refusal was rewritten:\n got %s\nwant %s",
			mismatch.Error(), said)
	}
}

// And every other refusal is left exactly as it was.
func TestAnOrdinaryRefusalIsNotAMismatch(t *testing.T) {
	err := asMismatch(wrap("node.add", &control.Coded{
		Code: control.NotFound, Err: errors.New("no node named \"Bishop Hill\""),
	}))
	var mismatch *ProtocolMismatch
	if errors.As(err, &mismatch) {
		t.Fatalf("a not-found refusal was reported as a version mismatch: %v", err)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("the refusal lost its kind: %v", err)
	}
}

// The mismatch this client notices itself still reads the way it always did:
// the workbench answered session.hello with a number, rather than refusing.
func TestAMismatchThisClientNoticedSaysBothNumbers(t *testing.T) {
	e := &ProtocolMismatch{
		Client:    1,
		Workbench: Hello{Protocol: 2, Version: "v9.9.9", Socket: "/run/mb.sock"},
	}
	for _, want := range []string{"1", "2", "v9.9.9", "/run/mb.sock", "Upgrade"} {
		if !strings.Contains(e.Error(), want) {
			t.Errorf("the mismatch does not say %q: %s", want, e.Error())
		}
	}
}
