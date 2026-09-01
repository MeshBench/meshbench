package control

import (
	"encoding/json"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// A version this build cannot speak is refused at the door, on both transports,
// and the refusal is about the version rather than about the verb.
//
// The verb is the whole point: before this, two ends that disagreed about the
// wire found out from whichever verb behaved oddly first, which in a CI run
// reads as a firmware regression rather than as a client that needs upgrading.

// declares opens a raw connection and sends one request with a wire version on
// it, returning what came back. Raw rather than through Client, because Client
// declares the version this build speaks and the interesting case is a client
// that does not.
func declares(t *testing.T, path string, req Request) Response {
	t.Helper()
	c, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	if err := json.NewEncoder(c).Encode(req); err != nil {
		t.Fatal(err)
	}
	var resp Response
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := json.NewDecoder(c).Decode(&resp); err != nil {
		t.Fatalf("nothing came back: %v", err)
	}
	return resp
}

func TestAClientSpeakingAnotherProtocolIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "version.sock")
	serveAt(t, path)

	resp := declares(t, path, Request{ID: 7, Method: "who", Protocol: Protocol + 1})
	if resp.Code != string(ProtocolMismatch) {
		t.Fatalf("code %q, want %q (%s)", resp.Code, ProtocolMismatch, resp.Error)
	}
	if resp.ID != 7 {
		t.Errorf("the refusal answers id %d, not the request's 7", resp.ID)
	}
	// Both numbers, or whoever reads it cannot tell which end is wrong.
	for _, want := range []string{
		strconv.Itoa(Protocol + 1), strconv.Itoa(Protocol), "Upgrade",
	} {
		if !contains(resp.Error, want) {
			t.Errorf("the refusal does not say %q: %s", want, resp.Error)
		}
	}
}

// The ordering is the whole mechanism: a mismatch has to be answered before the
// verb is dispatched, or the client is back to reading a verb failure and
// guessing.
func TestAMismatchIsAnsweredBeforeTheVerbIs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ordering.sock")
	serveAt(t, path)

	// "missing" is a verb this handler answers with NotFound, and "nonsense" is
	// one it does not have at all. Both must come back as a version error.
	for _, verb := range []string{"missing", "nonsense", SubscribeMethod} {
		resp := declares(t, path, Request{ID: 1, Method: verb, Protocol: Protocol + 1})
		if resp.Code != string(ProtocolMismatch) {
			t.Errorf("%s answered with %q, want %q (%s)",
				verb, resp.Code, ProtocolMismatch, resp.Error)
		}
	}
}

// The version this build speaks is served exactly as it was, which is the half
// of this that must not change: every existing script is one of these.
func TestAMatchingClientIsUnaffected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "matching.sock")
	serveAt(t, path)

	if resp := declares(t, path,
		Request{ID: 1, Method: "who", Protocol: Protocol}); resp.Error != "" {
		t.Fatalf("a client speaking this version was refused: %s (%s)",
			resp.Error, resp.Code)
	}
	// And through the client, which declares it on the first frame of every
	// connection without being asked.
	c, err := DialAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	for i := range 3 {
		if _, err := c.Call("who", nil); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
}

// A script written before the field existed says nothing, and is still served:
// this is the wire the version number exists to protect, and refusing an
// undeclared client would break every one of them to enforce it.
func TestAClientThatDeclaresNothingIsStillServed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "silent.sock")
	serveAt(t, path)

	if resp := declares(t, path, Request{ID: 1, Method: "who"}); resp.Error != "" {
		t.Fatalf("a client that declared nothing was refused: %s (%s)",
			resp.Error, resp.Code)
	}
}

// Loopback declares on the line it already sends the token on, and is refused
// there - after the token, so a peer that has not proven it belongs here does
// not learn what this build is from the refusal.
func TestALoopbackClientSpeakingAnotherProtocolIsRefused(t *testing.T) {
	srv := tcpServer(t)
	raw, err := net.DialTimeout("tcp", srv.Address().Addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()

	enc, dec := json.NewEncoder(raw), json.NewDecoder(raw)
	if err := enc.Encode(hello{
		Token: srv.Address().Token, Protocol: Protocol + 1}); err != nil {
		t.Fatal(err)
	}
	var resp Response
	_ = raw.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("the refusal was not readable: %v", err)
	}
	if resp.Code != string(ProtocolMismatch) {
		t.Fatalf("code %q, want %q (%s)", resp.Code, ProtocolMismatch, resp.Error)
	}
	if !contains(resp.Error, strconv.Itoa(Protocol+1)) {
		t.Errorf("the refusal does not say what the client spoke: %s", resp.Error)
	}
}

// The wrong token is still the wrong token, whatever version the line claims:
// a peer that has not proven it belongs on this port learns nothing else.
func TestAWrongTokenOutranksTheVersion(t *testing.T) {
	srv := tcpServer(t)
	raw, err := net.DialTimeout("tcp", srv.Address().Addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()

	enc, dec := json.NewEncoder(raw), json.NewDecoder(raw)
	if err := enc.Encode(hello{
		Token: "0000000000000000", Protocol: Protocol + 1}); err != nil {
		t.Fatal(err)
	}
	var resp Response
	_ = raw.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := dec.Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Code != string(Unauthorised) {
		t.Fatalf("code %q, want %q", resp.Code, Unauthorised)
	}
}

// Both remedies, because they are entirely different pieces of work and the
// numbers alone do not say which one a reader is looking at. The older-client
// case cannot yet happen on the wire - the first version is 1 and 0 means a
// client that did not say - so the sentence is checked here rather than left
// until the day the number moves.
func TestTheRefusalSaysWhichEndToMove(t *testing.T) {
	older := protocolRefusal(2, 3)
	if !contains(older.Error, "Upgrade this client") {
		t.Errorf("a client behind the workbench is not told to upgrade: %s", older.Error)
	}
	newer := protocolRefusal(3, 2)
	if !contains(newer.Error, "Upgrade the workbench") {
		t.Errorf("a client ahead of the workbench is not told to upgrade it: %s",
			newer.Error)
	}
	if older.Code != string(ProtocolMismatch) || newer.Code != string(ProtocolMismatch) {
		t.Errorf("a version refusal is not classified as one: %q, %q",
			older.Code, newer.Code)
	}
}

// The client half: a workbench can only refuse a version it was told about, so
// this client says what it speaks on the first frame of every connection and on
// no other - the answer cannot change while the connection is open.
func TestTheClientDeclaresItsVersionOnceAndFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "declared.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	frames := make(chan Request, 2)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		dec, enc := json.NewDecoder(c), json.NewEncoder(c)
		for {
			var req Request
			if dec.Decode(&req) != nil {
				return
			}
			frames <- req
			if enc.Encode(Response{ID: req.ID}) != nil {
				return
			}
		}
	}()

	c, err := DialAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	for range 2 {
		if _, err := c.Call("who", nil); err != nil {
			t.Fatal(err)
		}
	}

	if first := <-frames; first.Protocol != Protocol {
		t.Errorf("the first frame declares %d, want %d", first.Protocol, Protocol)
	}
	if second := <-frames; second.Protocol != 0 {
		t.Errorf("the version is repeated on frame two: %d", second.Protocol)
	}
}

func TestTheCompatibilityRuleIsExactMatch(t *testing.T) {
	for spoken, want := range map[int]bool{
		0: true, Protocol: true, Protocol + 1: false, Protocol + 7: false,
	} {
		if got := speaksProtocol(spoken); got != want {
			t.Errorf("a client speaking %d is served=%v, want %v", spoken, got, want)
		}
	}
}
