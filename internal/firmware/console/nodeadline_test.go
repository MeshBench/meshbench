// A port whose reads cannot be given a deadline still answers.
//
// os.File returns ErrNoDeadline for anything the runtime poller will not take,
// so this is a real shape rather than a defensive invention. The interesting
// property is not that such a port is fast - it cannot be - but that waiting
// on it never becomes waiting forever: the console has to bound the wait
// itself when nothing else will end the read.
package console_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware/console"
)

// deafPort never answers and refuses a deadline, so nothing can interrupt a
// read against it. The worst port the console can be handed.
type deafPort struct {
	name    string
	release chan struct{}
}

func newDeafPort(name string) *deafPort {
	return &deafPort{name: name, release: make(chan struct{})}
}

func (d *deafPort) Name() string { return d.name }

func (d *deafPort) SetReadDeadline(time.Time) error { return os.ErrNoDeadline }

func (d *deafPort) Write(p []byte) (int, error) { return len(p), nil }

func (d *deafPort) Read(p []byte) (int, error) {
	<-d.release
	return 0, os.ErrClosed
}

// Close is what finally frees the blocked read, so the test can leave nothing
// behind even though the console could not.
func (d *deafPort) Close() error {
	close(d.release)
	return nil
}

// The wait is bounded by the console rather than by the port, so a command to
// a port that can neither answer nor be interrupted still comes back.
//
// Without the fallback this is not a slow test, it is a hung one: the console
// would wait on a read that nothing will ever end.
func TestAPortThatCannotBeGivenADeadlineStillTimesOut(t *testing.T) {
	c := console.New()
	c.Timeout = 150 * time.Millisecond
	d := newDeafPort("deaf")
	if err := c.Attach(d); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })

	began := time.Now()
	r := c.Send(context.Background(), "deaf", "get freq")
	took := time.Since(began)

	if r.Err == nil {
		t.Fatal("a port that never answered reported no error")
	}
	if !strings.Contains(r.Err.Error(), "no prompt within") {
		t.Errorf("error was %q, wanted it to say no prompt arrived", r.Err)
	}
	// Generously bounded: the point is that it returned at all, not what it
	// cost. A machine under load can take far longer than the timeout without
	// this being the failure the test is looking for.
	if took > 10*time.Second {
		t.Errorf("took %s, so the wait was not bounded by the console", took)
	}
}

// The same port, given up on by its caller rather than timing out.
//
// This is the case the read deadline normally handles: the console frees the
// read and waits for the goroutine. It must not do that here, because nothing
// would ever free it and the wait would never end.
func TestCancellingACommandToADeadlinelessPortReturns(t *testing.T) {
	c := console.New()
	c.Timeout = time.Hour // long enough that only the cancellation can end this
	d := newDeafPort("deaf")
	if err := c.Attach(d); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	began := time.Now()
	r := c.Send(ctx, "deaf", "get freq")
	took := time.Since(began)

	if !errors.Is(r.Err, context.Canceled) {
		t.Errorf("error was %v, wanted the cancellation", r.Err)
	}
	if took > 10*time.Second {
		t.Errorf("took %s: the cancelled call waited on a read nothing can end", took)
	}
}
