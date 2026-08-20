package session

import (
	"context"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// A progress update must not throw away the way to stop the job.
//
// state.Job carried a Cancel field for months that nothing could ever call,
// and this is why: job.progress replaced the whole row, and the callback
// reporting "412 of 500" has no way to rebuild a closure it never had. The
// handle survived exactly until the first tick of progress.
func TestProgressKeepsTheCancel(t *testing.T) {
	st := state.New(10)
	Register(st, &Sim{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	stopped := false
	if _, err := st.Do(ctx, "job.progress", state.Job{
		ID: "tiles", What: "fetching", Total: 500,
		Cancel: func() { stopped = true },
	}); err != nil {
		t.Fatal(err)
	}
	// Progress, as the fetch itself reports it: counts, no closure.
	if _, err := st.Do(ctx, "job.progress", state.Job{
		ID: "tiles", What: "fetching", Done: 412, Total: 500,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Do(ctx, "job.cancel", "tiles"); err != nil {
		t.Fatalf("cancel after a progress update: %v", err)
	}
	if !stopped {
		t.Fatal("the job reported progress and lost its cancel; " +
			"that is the bug that made state.Job.Cancel dead on arrival")
	}
}

// A job nobody left a handle on says so, rather than accepting the press and
// carrying on regardless.
func TestCancellingWhatCannotBeStoppedSaysSo(t *testing.T) {
	st := state.New(10)
	Register(st, &Sim{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	if _, err := st.Do(ctx, "job.progress", state.Job{ID: "links", What: "measuring"}); err != nil {
		t.Fatal(err)
	}
	_, err := st.Do(ctx, "job.cancel", "links")
	if err == nil {
		t.Fatal("cancelling an uncancellable job reported success")
	}
	if !strings.Contains(err.Error(), "cannot be stopped") {
		t.Fatalf("the refusal did not say why: %v", err)
	}
	if _, err := st.Do(ctx, "job.cancel", "nothing-like-this"); err == nil {
		t.Fatal("cancelling a job that is not running reported success")
	}
}

// Stopping is not instant, and the strip must not pretend it is: the row
// stays, saying what it is doing, until the work notices its context.
func TestACancelledJobSaysItIsStopping(t *testing.T) {
	st := state.New(10)
	Register(st, &Sim{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	if _, err := st.Do(ctx, "job.progress", state.Job{
		ID: "tiles", What: "fetching terrain tiles", Total: 9, Cancel: func() {},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Do(ctx, "job.cancel", "tiles"); err != nil {
		t.Fatal(err)
	}
	snap := st.Snapshot()
	var found bool
	for _, j := range snap.Jobs {
		if j.ID == "tiles" {
			found = true
			if !strings.HasPrefix(j.What, "stopping") {
				t.Fatalf("a cancelled job still reads %q", j.What)
			}
		}
	}
	if !found {
		t.Fatal("the job vanished on the press, which claims it has already stopped")
	}
}
