// The pairing rule: a released client drives the release it shipped with.
//
// The protocol number beside this file says whether two ends can understand
// each other's frames. It is bumped rarely and on purpose, so it cannot answer
// the other question a script actually has, which is whether the client in the
// virtualenv is the one that came with the workbench on the PATH. Two releases
// apart with no protocol bump between them will connect happily and then
// disagree about a verb's parameters, and that failure surfaces forty calls
// later looking like the simulation misbehaving.
//
// So the release travels on the wire beside the protocol number, on the frame a
// client was already sending, and the workbench refuses a pair that cannot be
// one. Refused here rather than in the clients, because a third-party script
// speaking the raw socket is entitled to the same answer as a script using a
// shipped client.
package control

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/version"
	"github.com/MeshBench/meshbench/internal/diag"
)

// ourRelease is the release this build belongs to, read once.
//
// A package variable rather than a call at each use because a test cannot set
// a linker flag on the binary it is running inside, and a server that believes
// it is a release is the only way to exercise the refusal at all. Read into
// Server at ListenAt, so nothing on a connection's goroutine reads a variable a
// test might still be putting back.
var ourRelease = version.Release()

// pairs reports whether a client belonging to this release may drive a build
// belonging to that one.
//
// An exact match, or one of the two ends not being a release at all.
//
// The second half is what keeps the tree usable by the people working on it. A
// build from a working copy has no release stamped in it - version.Release is
// empty by design - and neither has a client run out of the same checkout, so a
// rule of "they must be equal" would refuse every pair a developer has, on
// every run, for a disagreement that does not exist. Nothing is lost by
// allowing it: what the rule exists to catch is a released client meeting a
// released workbench of another number, and both ends of that pair carry their
// stamp. Where one end is unstamped there is no second version to disagree
// with, only a build whose author is the one changing it.
//
// It also serves every script written against this socket before the field
// existed, which declares nothing and is indistinguishable from a working copy.
func pairs(spoken, ours string) bool {
	return spoken == "" || ours == "" || spoken == ours
}

// releaseRefusal is what a client from another release is told: which release
// each end belongs to, and that the remedy is to make them the same one.
//
// Both numbers and both roles, because "version mismatch" leaves a reader to
// work out which of the two things they have installed is the one to change,
// and they have to know that before they can do anything at all.
func releaseRefusal(spoken, ours string) Response {
	return Response{
		Error: fmt.Sprintf("this client is from MeshBench %s and this workbench "+
			"is MeshBench %s. A client and the workbench it drives must be the "+
			"same release: install the %s client, or run the %s workbench",
			spoken, ours, ours, spoken),
		Code: string(VersionMismatch),
	}
}

// notePairing says so when the release check did not compare anything, so a
// pair that was never checked is visible rather than quietly assumed sound.
func notePairing(spoken, ours string) {
	switch {
	case ours != "" && spoken == ours:
		return
	case ours == "" && spoken == "":
		diag.Printf("control", "release check skipped: neither this workbench "+
			"nor the client that connected is a release build")
	case ours == "":
		diag.Printf("control", "release check skipped: this workbench is a "+
			"development build; the client says it is %q", spoken)
	case spoken == "":
		diag.Printf("control", "release check skipped: the client named no "+
			"release; this workbench is %s", ours)
	}
}

// greeting checks the two things a client declares about itself and reports
// what to answer if either means the connection cannot go on: the wire version
// it speaks, and the release it belongs to.
//
// The protocol first. A client that cannot be understood at all is a worse
// disagreement than one that can be understood but should not be trusted, and
// its refusal names the thing to fix.
//
// noting is true only on the first frame of a connection, so a skipped check is
// said once rather than on every request.
func (s *Server) greeting(req Request, noting bool) (Response, bool) {
	if !speaksProtocol(req.Protocol) {
		return protocolRefusal(req.Protocol, Protocol), false
	}
	if !pairs(req.Release, s.release) {
		return releaseRefusal(req.Release, s.release), false
	}
	if noting {
		notePairing(req.Release, s.release)
	}
	return Response{}, true
}
