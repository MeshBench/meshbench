package control

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// answering is a server that describes itself the way a real session does, so
// a probe gets a whole row back rather than only proof that something is there.
func answering(t *testing.T, want string) *Server {
	t.Helper()
	srv, err := ListenAt(want, func(method string, _ json.RawMessage) (any, error) {
		if method != "session.hello" {
			return nil, nil
		}
		return map[string]any{"version": "v0.0.0-test", "mode": "headless",
			"project": "a blank network", "nodes": 3}, nil
	})
	if err != nil {
		t.Fatalf("ListenAt %s: %v", want, err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	go func() {
		for {
			select {
			case <-time.After(time.Millisecond):
				srv.Pump()
			case <-t.Context().Done():
				return
			}
		}
	}()
	return srv
}

func TestTwoWorkbenchesAreListedApart(t *testing.T) {
	t.Setenv(SessionsEnv, t.TempDir())
	dir := t.TempDir()
	a := answering(t, filepath.Join(dir, "a.sock"))
	b := answering(t, filepath.Join(dir, "b.sock"))

	got, err := Sessions("")
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("listed %d sessions, want 2: %+v", len(got), got)
	}
	seen := map[string]Session{}
	for _, s := range got {
		seen[s.Address] = s
	}
	for _, srv := range []*Server{a, b} {
		s, ok := seen[srv.Path()]
		if !ok {
			t.Fatalf("%s is running and was not listed: %+v", srv.Path(), got)
		}
		if s.PID != os.Getpid() {
			t.Errorf("%s: pid %d, want %d", s.Address, s.PID, os.Getpid())
		}
		if s.StartedAt.IsZero() {
			t.Errorf("%s: no start time, which is the field that tells two "+
				"otherwise identical runs apart", s.Address)
		}
		if s.Nodes != 3 || s.Project != "a blank network" || s.Mode != "headless" {
			t.Errorf("%s: described itself as %+v, want the session's own answer",
				s.Address, s.Detail)
		}
	}
}

// A row listed once must be listed the same way again, or a script cannot
// match a session it saw a moment ago against the same session now.
func TestTheListingIsOldestFirstAndStable(t *testing.T) {
	t.Setenv(SessionsEnv, t.TempDir())
	dir := t.TempDir()
	answering(t, filepath.Join(dir, "a.sock"))
	answering(t, filepath.Join(dir, "b.sock"))

	first, err := Sessions("")
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	second, err := Sessions("")
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	for i := range first {
		if first[i].Address != second[i].Address {
			t.Fatalf("row %d moved: %s then %s", i, first[i].Address, second[i].Address)
		}
	}
	if len(first) > 1 && first[0].StartedAt.After(first[1].StartedAt) {
		t.Errorf("listed newest first: %v then %v",
			first[0].StartedAt, first[1].StartedAt)
	}
}

// The whole point: a session that was killed rather than closed leaves its
// file and its socket behind, and neither is evidence of anything.
func TestAKilledSessionIsNotReportedAsRunning(t *testing.T) {
	reg := t.TempDir()
	t.Setenv(SessionsEnv, reg)
	dir := t.TempDir()
	live := answering(t, filepath.Join(dir, "live.sock"))

	// What SIGKILL leaves: a file naming an address, and a socket file at
	// that address with nothing behind it. Written by hand because a server
	// asked to close tidies up, and tidying up is the thing that cannot be
	// relied on.
	dead := filepath.Join(dir, "dead.sock")
	if err := os.WriteFile(dead, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(sessionFile{Address: dead, PID: os.Getpid(),
		StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	stale := sessionPath(reg, dead)
	if err := os.WriteFile(stale, body, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Sessions("")
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(got) != 1 || got[0].Address != live.Path() {
		t.Fatalf("listed %+v, want only the one that answers (%s)", got, live.Path())
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("%s survived a listing that found nothing behind it", stale)
	}
}

// A pid is not the check, and this is why: the file names a pid that exists
// and is this very process, and the session is still dead.
func TestAReusedPidDoesNotMakeASessionLive(t *testing.T) {
	reg := t.TempDir()
	t.Setenv(SessionsEnv, reg)
	body, err := json.Marshal(sessionFile{
		Address:   filepath.Join(t.TempDir(), "gone.sock"),
		PID:       os.Getpid(),
		StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reg, "stale.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Sessions("")
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("listed %+v for a pid that is alive and a socket that is not", got)
	}
}

func TestClosingRemovesTheRow(t *testing.T) {
	reg := t.TempDir()
	t.Setenv(SessionsEnv, reg)
	srv := answering(t, filepath.Join(t.TempDir(), "a.sock"))
	if _, err := os.Stat(sessionPath(reg, srv.Path())); err != nil {
		t.Fatalf("a listening server left no row: %v", err)
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(sessionPath(reg, srv.Path())); !os.IsNotExist(err) {
		t.Errorf("the row outlived the server it named: %v", err)
	}
}

// These files say where a control socket is, so they are nobody else's
// business - the same discipline the rendezvous file keeps.
func TestTheRowsAreReadableOnlyByTheirOwner(t *testing.T) {
	reg := t.TempDir()
	t.Setenv(SessionsEnv, reg)
	srv := answering(t, filepath.Join(t.TempDir(), "a.sock"))
	isPrivate(t, sessionPath(reg, srv.Path()), fs.FileMode(0o600))
	isPrivate(t, reg, fs.FileMode(0o700))
}

// A session cannot answer its own hello from inside a verb, so it is listed
// without being asked. It must still be listed.
func TestASessionListsItselfWithoutDiallingItself(t *testing.T) {
	t.Setenv(SessionsEnv, t.TempDir())
	srv := answering(t, filepath.Join(t.TempDir(), "a.sock"))
	got, err := Sessions(srv.Path())
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(got) != 1 || got[0].Address != srv.Path() {
		t.Fatalf("listed %+v, want its own row", got)
	}
	if got[0].Detail != (Detail{}) {
		t.Errorf("its own row was described from outside: %+v", got[0].Detail)
	}
	if got[0].PID != os.Getpid() || got[0].StartedAt.IsZero() {
		t.Errorf("its own row lost what the file knows: %+v", got[0])
	}
}

// A TCP session carries its own token, because two of them share one
// rendezvous file and the second overwrites the first.
func TestATCPSessionIsFoundWithItsOwnToken(t *testing.T) {
	t.Setenv(SessionsEnv, t.TempDir())
	t.Setenv(RendezvousEnv, filepath.Join(t.TempDir(), "control.json"))
	srv := answering(t, "tcp")

	// A second listener overwrites the shared rendezvous file, which is
	// exactly the case the token on the row exists to survive.
	t.Setenv(RendezvousEnv, filepath.Join(t.TempDir(), "other.json"))
	answering(t, "tcp")

	got, err := Sessions("")
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	var row Session
	for _, s := range got {
		if s.Address == srv.Path() {
			row = s
		}
	}
	if row.Address == "" {
		t.Fatalf("the first TCP session was not listed: %+v", got)
	}
	if row.Token == "" {
		t.Fatal("the row carries no token, so nothing could connect to it")
	}
	c, err := row.Dial()
	if err != nil {
		t.Fatalf("dialling the row: %v", err)
	}
	_ = c.Close()
}

// The token says where a control socket is and how to drive it. It belongs in
// the 0600 file it came from and nowhere a reply could carry it.
func TestARowNeverMarshalsItsToken(t *testing.T) {
	body, err := json.Marshal(Session{Address: "tcp:127.0.0.1:1", Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "secret") {
		t.Fatalf("a marshalled row carried the token: %s", body)
	}
}
