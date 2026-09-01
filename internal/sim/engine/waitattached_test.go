// waitAttached is unexported, so this stays in package engine rather than
// engine_test: the point of these is the fail-fast path itself, which
// nothing outside the package can reach.
package engine

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
)

// fakeExiterBackend is a Backend that can say whether its process has gone,
// the way the native backend does - without spawning a real process to prove
// it. exited is closed by the test to say the process is gone; left open, it
// stands in for one that is still running, the way a healthy native node's
// would be for as long as it is attached.
type fakeExiterBackend struct {
	exited chan struct{}
	err    error
	tail   string
}

func newFakeExiterBackend(err error, tail string) *fakeExiterBackend {
	return &fakeExiterBackend{exited: make(chan struct{}), err: err, tail: tail}
}

func (*fakeExiterBackend) Start(context.Context, string) error { return nil }
func (*fakeExiterBackend) Stop() error                         { return nil }
func (*fakeExiterBackend) Kind() string                        { return "fake-exiter" }
func (*fakeExiterBackend) HasConsole() bool                    { return false }
func (*fakeExiterBackend) ConsoleIn() io.Writer                { return nil }

func (b *fakeExiterBackend) Exited() <-chan struct{} { return b.exited }
func (b *fakeExiterBackend) ExitError() error        { return b.err }
func (b *fakeExiterBackend) StderrTail() string      { return b.tail }

// noExiterBackend implements Backend and nothing more - the shape of the
// emulated backend from waitAttached's point of view: it has no exec.Cmd to
// ask about, so asserting it against exiter must fail rather than panic.
type noExiterBackend struct{}

func (noExiterBackend) Start(context.Context, string) error { return nil }
func (noExiterBackend) Stop() error                         { return nil }
func (noExiterBackend) Kind() string                        { return "no-exiter" }
func (noExiterBackend) HasConsole() bool                    { return false }
func (noExiterBackend) ConsoleIn() io.Writer                { return nil }

func TestWaitAttachedFailsFastOnAProcessAlreadyGone(t *testing.T) {
	br, err := firmware.Listen("127.0.0.1:0", "gone")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = br.Close() }()

	backend := newFakeExiterBackend(
		errors.New("exit status 127"), "libc.so.6: version `GLIBC_2.38' not found")
	close(backend.exited)
	n := &firmware.Node{Bridge: br, Backend: backend}

	const budget = 30 * time.Second
	start := time.Now()
	err = waitAttached(context.Background(), n, budget)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("waitAttached returned nil for a process that was already gone")
	}
	if !strings.Contains(err.Error(), "127") {
		t.Errorf("error does not name the exit status: %v", err)
	}
	if !strings.Contains(err.Error(), "GLIBC_2.38") {
		t.Errorf("error does not quote the stderr tail: %v", err)
	}
	// Generous next to the 30s budget: this is "did it fail fast", not a
	// benchmark, and a loaded CI runner still finishes this in well under a
	// second of real work.
	if elapsed > 5*time.Second {
		t.Errorf("waitAttached took %v to report an already-dead process; budget was %v", elapsed, budget)
	}
}

func TestWaitAttachedIsUnaffectedByANormalAttach(t *testing.T) {
	br, err := firmware.Listen("127.0.0.1:0", "attaches")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = br.Close() }()

	conn, err := net.Dial("tcp", br.Addr())
	if err != nil {
		t.Fatalf("dial bridge: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// exited is never closed, standing in for a process that is still
	// running - the ordinary case, exercised through the same select the
	// fail-fast path uses rather than a backend that skips it entirely.
	n := &firmware.Node{Bridge: br, Backend: newFakeExiterBackend(nil, "")}
	waitForAttach(t, br)

	if err := waitAttached(context.Background(), n, 5*time.Second); err != nil {
		t.Fatalf("waitAttached on a normally attached node: %v", err)
	}
}

func TestWaitAttachedTimesOutWithoutAnExiter(t *testing.T) {
	br, err := firmware.Listen("127.0.0.1:0", "silent")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = br.Close() }()

	n := &firmware.Node{Bridge: br, Backend: noExiterBackend{}}

	const timeout = 50 * time.Millisecond
	start := time.Now()
	err = waitAttached(context.Background(), n, timeout)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("waitAttached returned nil for a node that never attached")
	}
	if !strings.Contains(err.Error(), "never connected") {
		t.Errorf("error does not describe a connect timeout: %v", err)
	}
	if elapsed < timeout {
		t.Errorf("waitAttached returned after %v, before its own %v timeout", elapsed, timeout)
	}
}

func waitForAttach(t *testing.T, br *firmware.Bridge) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !br.Attached() {
		if time.Now().After(deadline) {
			t.Fatal("bridge never reported a connection attached")
		}
		time.Sleep(time.Millisecond)
	}
}
