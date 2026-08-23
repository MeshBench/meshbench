package control

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"testing"
	"time"
)

// The transport Windows uses, exercised here.
//
// Windows has no AF_UNIX a Python client can reach, so it gets loopback TCP
// with a token. That path would otherwise be code nobody on this project ever
// runs - the lab is Linux and so is CI - which is the definition of code that
// is broken and nobody knows. So it is selectable everywhere, and these run it.

// tcpServer starts one on an ephemeral loopback port, with its own rendezvous
// file so tests do not fight over the real one.
func tcpServer(t *testing.T) *Server {
	t.Helper()
	// UserCacheDir is where the rendezvous lives, and pointing it at a
	// temporary directory is how a test gets one of its own on every platform.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv(SocketEnv, "")

	srv, err := ListenAt("tcp", func(method string, _ json.RawMessage) (any, error) {
		if method == "who" {
			return map[string]any{"ok": true}, nil
		}
		return nil, errors.New("unhandled")
	})
	if err != nil {
		t.Fatalf("listening on loopback: %v", err)
	}
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

func TestLoopbackCarriesTheSameProtocol(t *testing.T) {
	srv := tcpServer(t)
	if srv.Address().Kind != TCP {
		t.Fatalf("asked for tcp and got %v", srv.Address().Kind)
	}

	c, err := DialAt("tcp")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.Close() }()

	raw, err := c.Call("who", nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var got struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(raw, &got); err != nil || !got.OK {
		t.Fatalf("the answer did not survive the transport: %s (%v)", raw, err)
	}
}

// A port is bound, and it is bound where nothing outside this machine can
// reach it. ADR-0005 stands: nothing here is on the network.
func TestLoopbackOnly(t *testing.T) {
	srv := tcpServer(t)
	host, _, err := net.SplitHostPort(srv.Address().Addr)
	if err != nil {
		t.Fatal(err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		t.Fatalf("bound to %s, which is not loopback", host)
	}
}

// The token is the whole of the access control on a loopback port, because any
// local process can connect to one. A connection that does not present it is
// told so and closed.
func TestAConnectionWithoutTheTokenIsRefused(t *testing.T) {
	srv := tcpServer(t)

	raw, err := net.DialTimeout("tcp", srv.Address().Addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()

	// Straight into a request, with no token first - which is what a program
	// that found the port by scanning would do.
	enc, dec := json.NewEncoder(raw), json.NewDecoder(raw)
	if err := enc.Encode(Request{ID: 1, Method: "who"}); err != nil {
		t.Fatal(err)
	}
	var resp Response
	_ = raw.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("the refusal was not readable: %v", err)
	}
	if resp.Code != string(Unauthorised) {
		t.Fatalf("code %q, want %q (%s)", resp.Code, Unauthorised, resp.Error)
	}
}

func TestAWrongTokenIsRefused(t *testing.T) {
	srv := tcpServer(t)
	raw, err := net.DialTimeout("tcp", srv.Address().Addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()

	enc, dec := json.NewEncoder(raw), json.NewDecoder(raw)
	if err := enc.Encode(hello{Token: "0000000000000000"}); err != nil {
		t.Fatal(err)
	}
	var resp Response
	_ = raw.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("the refusal was not readable: %v", err)
	}
	if resp.Code != string(Unauthorised) {
		t.Fatalf("code %q, want %q", resp.Code, Unauthorised)
	}
}

// The rendezvous file is how a client finds an ephemeral port, and it is the
// thing holding the token - so it must not be readable by anybody else.
func TestTheRendezvousFileIsPrivate(t *testing.T) {
	tcpServer(t)
	path, err := RendezvousPath()
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("no address file was written: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("the file holding the token is %04o, want 0600", perm)
	}
}

// Closing takes the file with it: a stale one names a port nobody holds, and
// the next client would either fail or reach whatever has since taken it.
func TestClosingRemovesTheRendezvous(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv(SocketEnv, "")

	srv, err := ListenAt("tcp", func(string, json.RawMessage) (any, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	path, err := RendezvousPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("no address file while running: %v", err)
	}
	_ = srv.Close()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the address file outlived the workbench: %v", err)
	}
}

// A unix socket asks for no token, and must not start doing so: every script
// written against the socket so far would break, tools/soak included.
func TestAUnixSocketNeedsNoToken(t *testing.T) {
	path := t.TempDir() + "/plain.sock"
	serveAt(t, path)

	raw, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()

	enc, dec := json.NewEncoder(raw), json.NewDecoder(raw)
	if err := enc.Encode(Request{ID: 1, Method: "who"}); err != nil {
		t.Fatal(err)
	}
	var resp Response
	_ = raw.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("a plain request over a unix socket was not answered: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("it was refused: %s (%s)", resp.Error, resp.Code)
	}
}

// The socket's permissions are what a unix address has instead of a token.
func TestAUnixSocketIsPrivate(t *testing.T) {
	path := t.TempDir() + "/perms.sock"
	serveAt(t, path)
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("the socket is %04o, want 0600", perm)
	}
}
