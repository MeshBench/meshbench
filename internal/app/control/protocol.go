// The wire version, and what a client speaking a different one is told.
//
// The number on its own was a claim: it was declared here, reported by
// session.hello, and compared by whichever client remembered to look. A client
// that did not look, or looked after it had already sent something, found out
// from a verb behaving oddly - and a verb behaving oddly in a CI run reads as a
// firmware regression rather than as two ends that cannot speak to each other.
//
// So the declaration travels on the wire, on the frame a client was already
// sending, and the server refuses a version it does not speak before any verb
// is dispatched. No extra round trip and no new frame: an opening handshake of
// its own would have broken every script written against this socket before it
// existed, which is the wire this exists to protect.
package control

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/MeshBench/meshbench/internal/app/version"
)

// Protocol is the wire version, bumped only when a change breaks a client
// written against the previous number.
//
// Here rather than beside session.hello, which is what reports it: this is the
// package that defines the wire, and a client should be able to know what it
// speaks without importing a simulator.
//
// Adding a verb does not break anybody, and neither does adding a field to a
// result: a client reads the fields it knows. What moves this is a verb
// changing what it means, a field changing type, or the framing changing.
const Protocol = 1

// speaksProtocol reports whether a client that declared this number can be
// served.
//
// An exact match, both directions, and deliberately not "a newer workbench can
// still serve an older client": the number moves only when something a client
// written against the previous one relied on has changed, so any difference at
// all is that break by construction. Letting one direction through would be a
// rule about how careful the last bump happened to be, which is not something
// a connection can check. It is also the rule all three shipped clients
// already apply to session.hello, so the two ends never disagree about whether
// they agree.
//
// Zero is a client that did not say, and it is served. Every script written
// against this socket before the field existed is one of those; they find out
// from session.hello, which is what the clients have always done.
func speaksProtocol(spoken int) bool {
	return spoken == 0 || spoken == Protocol
}

// protocolRefusal is what a client speaking another version is told: both
// numbers, and which end has to move. Which end is worth a sentence, because
// the two remedies are entirely different pieces of work and two bare numbers
// leave a reader to work out which of them they are looking at.
//
// It formats a disagreement between two numbers and reads no global, so the
// half of the sentence that only happens once this build is the older one can
// be exercised on the day the change is written rather than on the day it
// first matters.
func protocolRefusal(spoken, speaks int) Response {
	build := version.String()
	move := "Upgrade this client to the one that ships with " + build
	if spoken > speaks {
		move = "Upgrade the workbench: " + build +
			" is older than the client driving it"
	}
	return Response{
		Error: fmt.Sprintf("this client speaks control protocol %d and this "+
			"workbench (%s) speaks %d. %s", spoken, build, speaks, move),
		Code: string(ProtocolMismatch),
	}
}

// welcomed reads the opening frame of a TCP connection and decides whether
// there is anything to go on with: the token, and then the version.
//
// The token first, so a peer that has not proven it may be here does not learn
// what this build is from the refusal.
func (s *Server) welcomed(c net.Conn, dec *json.Decoder, enc *json.Encoder) bool {
	h, ok := s.authorised(c, dec, enc)
	if !ok {
		return false
	}
	if !speaksProtocol(h.Protocol) {
		_ = enc.Encode(protocolRefusal(h.Protocol, Protocol))
		return false
	}
	return true
}
