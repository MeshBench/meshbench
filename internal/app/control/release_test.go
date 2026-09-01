package control

import (
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"
)

// The pairing rule, enforced by the workbench rather than by client politeness.
//
// A third-party script speaking the raw socket has to get the same answer as
// one using a shipped client, which is the whole reason the release travels on
// the wire instead of being compared only in the three clients.

// asRelease makes the next server built believe it is that release, and puts
// the real answer back once it has been built. A linker flag cannot be set on
// the binary a test is running inside, and the server reads its release once at
// ListenAt, so this is the seam - taken before anything is listening and
// released before anything connects, so no connection goroutine ever sees it
// move.
func asRelease(t *testing.T, r string) func() {
	t.Helper()
	was := ourRelease
	ourRelease = r
	return func() { ourRelease = was }
}

// servedAsRelease is serveAt, with the server believing it is a release.
func servedAsRelease(t *testing.T, path, release string) {
	t.Helper()
	restore := asRelease(t, release)
	serveAt(t, path)
	restore()
}

func TestAClientFromAnotherReleaseIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "release.sock")
	servedAsRelease(t, path, "2.0.0")

	resp := declares(t, path,
		Request{ID: 4, Method: "who", Protocol: Protocol, Release: "1.5.0"})
	if resp.Code != string(VersionMismatch) {
		t.Fatalf("code %q, want %q (%s)", resp.Code, VersionMismatch, resp.Error)
	}
	if resp.ID != 4 {
		t.Errorf("the refusal answers id %d, not the request's 4", resp.ID)
	}
	// Both releases and the remedy: a bare "version mismatch" leaves a reader
	// to work out which of the two things they have installed to change, and
	// nobody can act until they know.
	for _, want := range []string{"1.5.0", "2.0.0", "must be the same release"} {
		if !contains(resp.Error, want) {
			t.Errorf("the refusal does not say %q: %s", want, resp.Error)
		}
	}
}

// The verb is the point. A pair that was never built together has to be turned
// away before anything is dispatched, or the script is back to reading a verb
// failure and guessing at what caused it.
func TestAReleaseMismatchIsAnsweredBeforeTheVerbIs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "releaseorder.sock")
	servedAsRelease(t, path, "2.0.0")

	for _, verb := range []string{"who", "missing", "nonsense", SubscribeMethod} {
		resp := declares(t, path,
			Request{ID: 1, Method: verb, Protocol: Protocol, Release: "1.5.0"})
		if resp.Code != string(VersionMismatch) {
			t.Errorf("%s answered with %q, want %q (%s)",
				verb, resp.Code, VersionMismatch, resp.Error)
		}
	}
}

// The same release is the pair the rule exists to allow, and it must be served
// exactly as it was.
func TestAClientFromTheSameReleaseIsServed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "samerelease.sock")
	servedAsRelease(t, path, "2.0.0")

	if resp := declares(t, path,
		Request{ID: 1, Method: "who", Protocol: Protocol, Release: "2.0.0"}); resp.Error != "" {
		t.Fatalf("a client from this release was refused: %s (%s)",
			resp.Error, resp.Code)
	}
}

// A build from a working copy stamps no release, and neither does a client run
// out of the same checkout. Refusing those would make the tree unusable by the
// people changing it, for a disagreement that does not exist: there is only one
// build, and its author is the one editing it.
func TestADevelopmentBuildDoesNotRefuseAnybody(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devbuild.sock")
	serveAt(t, path) // no stamp, which is what a working copy produces

	for _, spoken := range []string{"", "1.5.0", "99.0.0"} {
		resp := declares(t, path,
			Request{ID: 1, Method: "who", Protocol: Protocol, Release: spoken})
		if resp.Error != "" {
			t.Errorf("a development build refused a client saying %q: %s (%s)",
				spoken, resp.Error, resp.Code)
		}
	}
}

// The other half: a released workbench serves a client that names no release,
// which is every script written against this socket before the field existed.
func TestAClientThatNamesNoReleaseIsServed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unnamed.sock")
	servedAsRelease(t, path, "2.0.0")

	if resp := declares(t, path,
		Request{ID: 1, Method: "who", Protocol: Protocol}); resp.Error != "" {
		t.Fatalf("a client that named no release was refused: %s (%s)",
			resp.Error, resp.Code)
	}
}

// Loopback declares on the line it already sends the token on, and is refused
// there, so the rule is not one that only applies over a unix socket.
func TestALoopbackClientFromAnotherReleaseIsRefused(t *testing.T) {
	restore := asRelease(t, "2.0.0")
	srv := tcpServer(t)
	restore()

	raw, err := net.DialTimeout("tcp", srv.Address().Addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()

	enc, dec := json.NewEncoder(raw), json.NewDecoder(raw)
	if err := enc.Encode(hello{
		Token: srv.Address().Token, Protocol: Protocol, Release: "1.5.0"}); err != nil {
		t.Fatal(err)
	}
	var resp Response
	_ = raw.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("the refusal was not readable: %v", err)
	}
	if resp.Code != string(VersionMismatch) {
		t.Fatalf("code %q, want %q (%s)", resp.Code, VersionMismatch, resp.Error)
	}
	for _, want := range []string{"1.5.0", "2.0.0"} {
		if !contains(resp.Error, want) {
			t.Errorf("the refusal does not say %q: %s", want, resp.Error)
		}
	}
}

// The client half: a workbench can only refuse a pair it was told about, so the
// release goes on the first frame of every connection and on no other.
func TestTheClientDeclaresItsReleaseOnceAndFirst(t *testing.T) {
	restore := asRelease(t, "2.0.0")
	defer restore()

	path := filepath.Join(t.TempDir(), "declaredrelease.sock")
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

	if first := <-frames; first.Release != "2.0.0" {
		t.Errorf("the first frame declares %q, want %q", first.Release, "2.0.0")
	}
	if second := <-frames; second.Release != "" {
		t.Errorf("the release is repeated on frame two: %q", second.Release)
	}
}

func TestThePairingRuleIsExactMatchOrAnUnstampedEnd(t *testing.T) {
	cases := []struct {
		spoken, ours string
		want         bool
	}{
		{"1.0.0", "1.0.0", true},
		{"1.0.0", "1.0.1", false},
		{"1.0.0", "2.0.0", false},
		{"", "1.0.0", true},
		{"1.0.0", "", true},
		{"", "", true},
	}
	for _, c := range cases {
		if got := pairs(c.spoken, c.ours); got != c.want {
			t.Errorf("a %q client against a %q workbench is served=%v, want %v",
				c.spoken, c.ours, got, c.want)
		}
	}
}
