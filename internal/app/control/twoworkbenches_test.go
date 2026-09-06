package control

import (
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// quiet is a handler that answers nothing, because these tests are about
// whether a listener comes up and where it is announced, not about verbs.
func quiet(string, json.RawMessage) (any, error) { return nil, nil }

// alone gives each test its own registry and its own address file, so two of
// them running at once do not see each other's sessions.
func alone(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(SessionsEnv, filepath.Join(dir, "sessions"))
	t.Setenv(RendezvousEnv, filepath.Join(dir, "control.json"))
}

// Two workbenches asking for any free port both get one.
//
// Reported from Windows, where tcp is the default: the second refused to start
// at all, with "tcp:127.0.0.1:0 is already answering". Port 0 is the sentinel
// meaning "whatever is free" - nothing can hold it, nothing is listening on it,
// and the operator never typed it. The check was really asking whether the
// rendezvous file was occupied, which is a different question and not one that
// should stop a workbench running.
func TestTwoEphemeralWorkbenchesBothStart(t *testing.T) {
	alone(t)

	first, err := ListenAt("tcp", Handler(quiet))
	if err != nil {
		t.Fatalf("the first workbench did not start: %v", err)
	}
	defer func() { _ = first.Close() }()

	second, err := ListenAt("tcp", Handler(quiet))
	if err != nil {
		t.Fatalf("the second workbench was refused: %v", err)
	}
	defer func() { _ = second.Close() }()

	if first.Address().Addr == second.Address().Addr {
		t.Fatalf("both took %s, so one is not really listening", first.Address().Addr)
	}
}

// A refusal names the address that is actually held.
//
// The old message printed the address that was asked for, so an ephemeral
// request was reported as a conflict over ":0" - an address nobody holds and
// nobody typed. When there is a genuine conflict the message has to name the
// thing somebody can act on.
func TestARefusalNamesTheAddressSomebodyElseHolds(t *testing.T) {
	alone(t)

	first, err := ListenAt("tcp", Handler(quiet))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()

	// Ask for the port the first one actually took.
	_, err = ListenAt("tcp:"+first.Address().Addr, Handler(quiet))
	if err == nil {
		t.Fatal("two listeners on one port were allowed")
	}
	if strings.Contains(err.Error(), ":0") {
		t.Errorf("the refusal names port 0, which nothing holds: %v", err)
	}
	if !strings.Contains(err.Error(), first.Address().Addr) {
		t.Errorf("the refusal does not name %s, the address actually held: %v",
			first.Address().Addr, err)
	}
}

// A second workbench does not take the discovery slot from a live first.
//
// control.json names the workbench a client with no address finds. Overwriting
// it left the first running, answering, and unreachable by every documented
// route - and nothing errored, so a script drove the wrong session and the
// numbers looked plausible because they came from a real engine.
func TestASecondWorkbenchDoesNotOrphanTheFirstFromDiscovery(t *testing.T) {
	alone(t)

	first, err := ListenAt("tcp", Handler(quiet))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()

	r, err := readRendezvous()
	if err != nil {
		t.Fatalf("the first workbench left no address: %v", err)
	}
	if r.Address != first.Address().Addr {
		t.Fatalf("the file names %s, not the first workbench's %s",
			r.Address, first.Address().Addr)
	}

	second, err := ListenAt("tcp", Handler(quiet))
	if err != nil {
		t.Fatalf("the second workbench did not start: %v", err)
	}
	defer func() { _ = second.Close() }()

	after, err := readRendezvous()
	if err != nil {
		t.Fatalf("the address file went missing: %v", err)
	}
	if after.Address != first.Address().Addr {
		t.Errorf("discovery now points at %s; the first workbench is running "+
			"at %s and can no longer be found", after.Address, first.Address().Addr)
	}
	// And the newcomer is still findable the way a client should be asking.
	got, err := Sessions("")
	if err != nil {
		t.Fatalf("listing the running sessions: %v", err)
	}
	var addrs []string
	for _, s := range got {
		addrs = append(addrs, s.Address)
	}
	if len(got) < 2 {
		t.Errorf("only %d session(s) listed, %v - a workbench that is not in "+
			"control.json has to be somewhere", len(got), addrs)
	}
}

// A first line carrying a request is answered, not swallowed.
//
// The handshake is its own line. A client that put its token inside its first
// call used to authorise, have that call read as the greeting, and then wait
// for a reply to something nothing had queued - so the connection hung, with
// no error at either end and nothing in any log to say why.
func TestAGreetingCarryingARequestIsRefusedRatherThanEaten(t *testing.T) {
	alone(t)
	srv, err := ListenAt("tcp", Handler(quiet))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()

	c, err := net.Dial("tcp", srv.Address().Addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	// The token in the right place, but on a line that is also a call.
	line := fmt.Sprintf(`{"id":1,"method":"sim.state","token":%q}`+"\n",
		srv.Address().Token)
	if _, err := c.Write([]byte(line)); err != nil {
		t.Fatal(err)
	}
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	var got Response
	if err := json.NewDecoder(c).Decode(&got); err != nil {
		t.Fatalf("nothing came back, so the connection hung: %v", err)
	}
	if got.Error == "" {
		t.Fatal("the request was accepted as a greeting and answered nothing")
	}
	if !strings.Contains(got.Error, "handshake") {
		t.Errorf("the refusal does not say what is wrong: %q", got.Error)
	}
}
