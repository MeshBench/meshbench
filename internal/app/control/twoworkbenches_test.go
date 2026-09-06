package control

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
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
	defer first.Close()

	second, err := ListenAt("tcp", Handler(quiet))
	if err != nil {
		t.Fatalf("the second workbench was refused: %v", err)
	}
	defer second.Close()

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
	defer first.Close()

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
	defer first.Close()

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
	defer second.Close()

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
