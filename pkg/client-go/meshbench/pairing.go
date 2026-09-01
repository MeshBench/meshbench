// What a connection agrees on before it is handed to a script: the wire
// version both ends speak, and the release both ends belong to.
//
// A client and the workbench it drives must be the same release. Two releases
// apart connect happily today, because the protocol number moves rarely and on
// purpose, and then disagree about a verb's parameters forty calls into a
// script - which in a CI run reads as a firmware regression rather than as two
// pieces of software that were never meant to be used together.
package meshbench

import (
	"context"

	"github.com/MeshBench/meshbench/internal/app/control"
	"github.com/MeshBench/meshbench/internal/app/version"
)

// greet asks what this is and refuses a build it cannot speak to.
//
// At the door rather than halfway through, and refused by both ends. The
// workbench has already turned away a version it will not serve, on the frame
// this client declared it on, so the comparisons here look redundant. They are
// not: a workbench old enough to predate the declaration ignores it and serves
// the connection anyway, and this end is then the only one left that can
// notice.
func (w *Workbench) greet(ctx context.Context) error {
	if err := w.CallInto(ctx, "session.hello", nil, &w.hello); err != nil {
		return asMismatch(err)
	}
	if w.hello.Protocol != control.Protocol {
		return &ProtocolMismatch{Client: control.Protocol, Workbench: w.hello}
	}
	ours := version.Release()
	if !pairedRelease(ours, w.hello.Release) {
		return &VersionMismatch{Client: ours, Workbench: w.hello.Release}
	}
	w.versionCheck = pairingNote(ours, w.hello.Release)
	return nil
}

// pairedRelease is the same rule the workbench applies, said again here because
// a build that predates the rule will not apply it at all.
//
// An end that names no release is served. That is what keeps the tree usable by
// the people working on it: a build from a working copy has no release stamped
// in it, nor has a client run out of the same checkout, so insisting on
// equality would refuse every pair a developer has for a disagreement that does
// not exist. Nothing is lost, because what the rule exists to catch is a
// released client meeting a released workbench of another number, and both ends
// of that pair carry their stamp.
func pairedRelease(ours, theirs string) bool {
	return ours == "" || theirs == "" || ours == theirs
}

// pairingNote is what to say about a check that did not compare anything, so a
// pair nothing verified is visible rather than assumed sound.
func pairingNote(ours, theirs string) string {
	switch {
	case ours == "" && theirs == "":
		return "release check skipped: neither this client nor the workbench is a release build"
	case ours == "":
		return "release check skipped: this client is a development build; the workbench is " + theirs
	case theirs == "":
		return "release check skipped: the workbench is a development build; this client is " + ours
	}
	return ""
}

// VersionCheck says what became of the release check at connect: empty when the
// two ends compared equal, and a sentence naming what was skipped and why when
// one of them was not a release build.
//
// Reported rather than logged, because a client is a library: a script that
// wants the line in its output can print it, and one that does not is not made
// noisy by a rule that did not apply to it.
func (w *Workbench) VersionCheck() string { return w.versionCheck }
