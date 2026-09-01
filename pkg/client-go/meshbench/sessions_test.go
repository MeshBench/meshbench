package meshbench

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/control"
)

// waitUntilGone blocks until nothing answers at a unix socket path.
func waitUntilGone(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.Dial("unix", path) //nolint:noctx // a local probe, bounded above
		if err != nil {
			return
		}
		_ = c.Close()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s was still answering 30s after its process was killed", path)
}

// twoRunning starts two workbenches at once, on a registry of their own.
//
// Its own registry because these tests remove what they find dead in it, and
// the machine running them may have somebody's workbench open.
func twoRunning(t *testing.T) (*Workbench, *Workbench, context.Context) {
	t.Helper()
	t.Setenv(control.SessionsEnv, t.TempDir())
	dir := t.TempDir()
	a, ctx := headless(t, Socket(filepath.Join(dir, "a.sock")))
	b, _ := headless(t, Socket(filepath.Join(dir, "b.sock")))
	return a, b, ctx
}

func TestTwoRunningWorkbenchesCanBeToldApart(t *testing.T) {
	a, b, _ := twoRunning(t)

	rows, err := Sessions()
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	by := map[string]Session{}
	for _, r := range rows {
		by[r.Address] = r
	}
	if len(by) != 2 {
		t.Fatalf("listed %d sessions, want 2: %+v", len(by), rows)
	}
	for _, wb := range []*Workbench{a, b} {
		r, ok := by[wb.Hello().Socket]
		if !ok {
			t.Fatalf("%s is running and was not listed: %+v", wb.Hello().Socket, rows)
		}
		// Everything a script needs in order to choose one: where, which
		// process, when it started, and what it is running.
		if r.PID != wb.Hello().PID {
			t.Errorf("%s: pid %d, want %d", r.Address, r.PID, wb.Hello().PID)
		}
		if r.StartedAt.IsZero() {
			t.Errorf("%s: no start time", r.Address)
		}
		if r.Mode != "headless" || r.Version == "" {
			t.Errorf("%s: described itself as %+v", r.Address, r.Detail)
		}
	}
	if a.Hello().PID == b.Hello().PID {
		t.Fatal("two workbenches with one pid, so nothing here means anything")
	}
}

func TestARowFromTheListingCanBeAttachedTo(t *testing.T) {
	a, _, ctx := twoRunning(t)
	rows, err := Sessions()
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	var row Session
	for _, r := range rows {
		if r.Address == a.Hello().Socket {
			row = r
		}
	}
	if row.Address == "" {
		t.Fatalf("%s was not listed: %+v", a.Hello().Socket, rows)
	}
	also, err := AttachTo(ctx, row)
	if err != nil {
		t.Fatalf("AttachTo: %v", err)
	}
	defer func() { _ = also.Close() }()
	if also.Hello().PID != a.Hello().PID {
		t.Errorf("attached to pid %d, want %d", also.Hello().PID, a.Hello().PID)
	}
}

func TestASessionListsTheOthersAndMarksItsOwnRow(t *testing.T) {
	a, b, ctx := twoRunning(t)
	rows, err := a.Sessions(ctx)
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("listed %d, want 2: %+v", len(rows), rows)
	}
	for _, r := range rows {
		if r.Self != (r.Address == a.Hello().Socket) {
			t.Errorf("%s: self is %v", r.Address, r.Self)
		}
		if r.Token != "" {
			t.Errorf("%s: a reply carried a token", r.Address)
		}
		want := a.Hello().Version
		if r.Address == b.Hello().Socket {
			want = b.Hello().Version
		}
		if r.Version != want {
			t.Errorf("%s: version %q, want %q", r.Address, r.Version, want)
		}
	}
}

// The one that matters. SIGKILL leaves the socket file and the row behind and
// gives the process no chance to tidy either up, so anything trusting what is
// on disk would report a dead session as running.
func TestAKilledWorkbenchIsNotReportedAsRunning(t *testing.T) {
	a, b, ctx := twoRunning(t)
	dead := b.Hello().Socket
	p, err := os.FindProcess(b.Hello().PID)
	if err != nil {
		t.Fatal(err)
	}
	// Kill rather than Close: the whole question is what happens when a
	// workbench is given no chance to tidy up after itself.
	if err := p.Kill(); err != nil {
		t.Fatalf("killing the second workbench: %v", err)
	}
	// Waited for by dialling rather than by reaping, because the process is
	// the client's to reap and taking it here would leave Close with nothing
	// to wait for. The kernel unbinds the listener when the process goes, so
	// a refused dial is the moment the claim below has to hold.
	waitUntilGone(t, dead)
	// The socket it bound is still on disk, which is exactly why a stat would
	// not do.
	if _, err := os.Stat(dead); err != nil {
		t.Fatalf("expected a leftover socket at %s: %v", dead, err)
	}

	rows, err := Sessions()
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(rows) != 1 || rows[0].Address != a.Hello().Socket {
		t.Fatalf("listed %+v, want only %s", rows, a.Hello().Socket)
	}
	seen, err := a.Sessions(ctx)
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(seen) != 1 || seen[0].Address != a.Hello().Socket {
		t.Fatalf("the session listed %+v, want only itself", seen)
	}
}
