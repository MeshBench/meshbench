package control

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// A socket per instance, and a live one that cannot be taken.
//
// One path per user was enough while the only caller was an operator's own
// desktop. Two CI jobs on one runner is the case that broke it, and the way it
// broke is the worst kind: the second process unlinked the first one's socket
// and bound its own, the first carried on with a listener nothing could reach,
// and neither end had any way to notice.

func serveAt(t *testing.T, path string) *Server {
	t.Helper()
	srv, err := ListenAt(path, func(method string, _ json.RawMessage) (any, error) {
		switch method {
		case "who":
			return map[string]any{"path": path}, nil
		case "missing":
			return nil, WithCode(NotFound, errors.New("no node named \"Bishop Hill\""))
		}
		return nil, errors.New("unhandled")
	})
	if err != nil {
		t.Fatalf("listen at %s: %v", path, err)
	}
	// Pumped here, because nothing outside a workbench has a frame loop.
	stop := make(chan struct{})
	go func() {
		tick := time.NewTicker(2 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				srv.Pump()
			}
		}
	}()
	t.Cleanup(func() { close(stop); _ = srv.Close() })
	return srv
}

func TestTwoWorkbenchesOnOneMachine(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.sock"), filepath.Join(dir, "b.sock")
	serveAt(t, a)
	serveAt(t, b)

	for _, want := range []string{a, b} {
		c, err := DialAt(want)
		if err != nil {
			t.Fatalf("dial %s: %v", want, err)
		}
		raw, err := c.Call("who", nil)
		if err != nil {
			t.Fatalf("call on %s: %v", want, err)
		}
		var got struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if got.Path != want {
			t.Fatalf("dialled %s and reached the workbench on %s", want, got.Path)
		}
		_ = c.Close()
	}
}

func TestALiveSocketIsNotTaken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "held.sock")
	serveAt(t, path)

	_, err := ListenAt(path, func(string, json.RawMessage) (any, error) {
		return nil, nil
	})
	if err == nil {
		t.Fatal("a second workbench took a socket another one was answering on")
	}
	// Named, because "address already in use" leaves somebody guessing which
	// of their two runs holds it.
	if !contains(err.Error(), path) {
		t.Errorf("the refusal does not name the path: %v", err)
	}

	// And the first one still answers.
	c, err := DialAt(path)
	if err != nil {
		t.Fatalf("the workbench that held the socket has lost it: %v", err)
	}
	if _, err := c.Call("who", nil); err != nil {
		t.Fatalf("it holds the socket and cannot answer on it: %v", err)
	}
	_ = c.Close()
}

// The case the removal was written for: a crashed run leaves a socket file
// behind with nothing behind it, and the next start must clean it up.
func TestAStaleSocketFileIsCleared(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stale.sock")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	serveAt(t, path)
	c, err := DialAt(path)
	if err != nil {
		t.Fatalf("a leftover file stopped a fresh workbench starting: %v", err)
	}
	_ = c.Close()
}

func TestAddressPrecedence(t *testing.T) {
	t.Setenv(SocketEnv, "/tmp/from-the-environment.sock")
	got, err := Resolve("/tmp/asked-for.sock")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != Unix || got.Addr != "/tmp/asked-for.sock" {
		t.Errorf("an explicit path lost to the environment: %v", got)
	}
	if got, _ = Resolve(""); got.Addr != "/tmp/from-the-environment.sock" {
		t.Errorf("the environment was ignored: %v", got)
	}
	t.Setenv(SocketEnv, "")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/test")
	if runtime.GOOS != "windows" {
		if got, _ = Resolve(""); got.Addr != "/run/user/test/meshbench.sock" {
			t.Errorf("the Linux default moved: %v", got)
		}
	}
}

// The forms an address can be written in, and the one that is refused.
func TestAddressForms(t *testing.T) {
	t.Setenv(SocketEnv, "")
	for _, c := range []struct {
		in   string
		kind Kind
		addr string
	}{
		{"/tmp/a.sock", Unix, "/tmp/a.sock"},
		{"unix:/tmp/b.sock", Unix, "/tmp/b.sock"},
		{"tcp", TCP, "127.0.0.1:0"},
		{"tcp:5599", TCP, "127.0.0.1:5599"},
		{"tcp:127.0.0.1:5599", TCP, "127.0.0.1:5599"},
	} {
		got, err := Resolve(c.in)
		if err != nil {
			t.Errorf("%s: %v", c.in, err)
			continue
		}
		if got.Kind != c.kind || got.Addr != c.addr {
			t.Errorf("%s resolved to %v, want %s %s", c.in, got, c.kind, c.addr)
		}
	}
}

// Loopback only, and said so rather than quietly narrowed: somebody who typed
// an outward-facing address meant something this program does not do.
func TestAnOutwardAddressIsRefused(t *testing.T) {
	t.Setenv(SocketEnv, "")
	for _, in := range []string{"tcp:0.0.0.0:5599", "tcp:192.168.1.10:5599"} {
		if _, err := Resolve(in); err == nil {
			t.Errorf("%s was accepted", in)
		}
	}
}

// A unix socket path has a hard limit and macOS temporary directories are long
// enough to reach it. The failure otherwise is a bind refusing with something
// about an invalid argument.
func TestAnOverlongUnixPathIsRefusedClearly(t *testing.T) {
	t.Setenv(SocketEnv, "")
	long := "/tmp/" + strings.Repeat("x", 200) + ".sock"
	_, err := Resolve(long)
	if err == nil {
		t.Fatal("a path past sun_path was accepted")
	}
	if !contains(err.Error(), "at most") {
		t.Errorf("the refusal does not say what the limit is: %v", err)
	}
}

// A code travels with the message, and the message is left alone.
func TestAnErrorCarriesItsCodeAndItsProse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codes.sock")
	serveAt(t, path)
	c, err := DialAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	_, err = c.Call("missing", nil)
	if err == nil {
		t.Fatal("a verb that refused reported success")
	}
	if got := CodeOf(err); got != NotFound {
		t.Errorf("code is %q, want %q", got, NotFound)
	}
	if !contains(err.Error(), "Bishop Hill") {
		t.Errorf("the verb's own words did not survive: %v", err)
	}
}

// An unknown verb is not an internal fault, and a client has to be able to
// tell: it means the build is older or newer than the client.
func TestAnUnclassifiedErrorIsInternal(t *testing.T) {
	if got := CodeOf(errors.New("something went wrong")); got != Internal {
		t.Errorf("an unclassified error is %q, want %q", got, Internal)
	}
	if got := CodeOf(nil); got != Unknown {
		t.Errorf("no error is %q, want empty", got)
	}
}

func TestClassifyMapsASentinel(t *testing.T) {
	boom := errors.New("the store has stopped")
	Classify(boom, Closing)
	if got := CodeOf(errors.New("wrapped")); got == Closing {
		t.Fatal("an unrelated error was classified")
	}
	wrapped := WithCode(Unknown, boom)
	if got := CodeOf(wrapped); got != Unknown {
		t.Errorf("an explicit code lost to a sentinel: %q", got)
	}
	if got := CodeOf(boom); got != Closing {
		t.Errorf("the registered sentinel is %q, want %q", got, Closing)
	}
}

// flakyListener stands in for a real one refusing Accept a fixed number of
// times before it starts working - what a process brushing a file descriptor
// ceiling looks like to whoever is calling Accept, with nothing distinguishing
// it from a fault at the type level.
type flakyListener struct {
	net.Listener
	mu    sync.Mutex
	fails int
}

func (f *flakyListener) Accept() (net.Conn, error) {
	f.mu.Lock()
	if f.fails > 0 {
		f.fails--
		f.mu.Unlock()
		return nil, errors.New("transient: too many open files")
	}
	f.mu.Unlock()
	return f.Listener.Accept()
}

// A transient Accept error must not be the last thing the accept loop ever
// does: it retries, with backoff, and the socket goes on answering once
// whatever briefly starved it clears.
func TestATransientAcceptErrorDoesNotDisableTheSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flaky.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	flaky := &flakyListener{Listener: ln, fails: 3}
	srv := &Server{
		ln:      flaky,
		addr:    Address{Kind: Unix, Addr: path},
		handler: func(string, json.RawMessage) (any, error) { return map[string]any{"ok": true}, nil },
	}
	go srv.accept()
	t.Cleanup(func() { _ = srv.Close() })

	stop := make(chan struct{})
	go func() {
		tick := time.NewTicker(2 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				srv.Pump()
			}
		}
	}()
	t.Cleanup(func() { close(stop) })

	// The listener refuses three times before it will ever hand back a
	// connection; a permanently disabled accept loop would never get through
	// this, backoff or not.
	deadline := time.Now().Add(3 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		c, err := DialAt(path)
		if err == nil {
			_, err = c.Call("who", nil)
			_ = c.Close()
			if err == nil {
				return
			}
		}
		lastErr = err
		time.Sleep(acceptBackoffMin)
	}
	t.Fatalf("the socket never recovered from transient accept errors: %v", lastErr)
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
