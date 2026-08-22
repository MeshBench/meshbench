// Waiting, in one place.
//
// Every wait in this package is a method, never a sleep in a caller. That is
// not tidiness: tools/soak hand-wrote the same poll loop three times in
// seventy-two lines, each with its own interval and its own timeout, and its
// own header records having sampled the wrong moment because of it.
//
// They poll. When the socket learns to push, this file changes and no caller
// does - which is the whole reason the clients are built before the events.
package client

import (
	"context"
	"fmt"
	"time"
)

// pollEvery is how often a wait asks.
//
// A tenth of a second, because these wait on things that take seconds to
// minutes - firmware coming up, a warm finishing, a run ending - and a faster
// poll would be a verb per frame against a store that has real work to do.
const pollEvery = 100 * time.Millisecond

// Timeout is a wait that ran out, saying what it was waiting for and what the
// state actually was.
//
// Not a bare deadline error: "timeout" in a CI log tells whoever reads it
// nothing, and the state at the moment it gave up is the only thing that does.
type Timeout struct {
	What  string
	After time.Duration
	// Last is what the final check saw.
	Last string
}

func (t *Timeout) Error() string {
	if t.Last == "" {
		return fmt.Sprintf("waited %s for %s", t.After, t.What)
	}
	return fmt.Sprintf("waited %s for %s; last saw: %s", t.After, t.What, t.Last)
}

// waitFor polls until check says yes, the context ends, or the time runs out.
//
// check returns whether it is done, what it saw if not, and an error that
// stops the wait rather than being retried - a verb refusing because the node
// does not exist will refuse the same way in ten seconds.
func waitFor(ctx context.Context, timeout time.Duration, what string,
	check func() (bool, string, error)) error {
	if timeout <= 0 {
		timeout = time.Minute
	}
	deadline := time.Now().Add(timeout)
	last := ""
	for {
		done, saw, err := check()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		if saw != "" {
			last = saw
		}
		if time.Now().After(deadline) {
			return &Timeout{What: what, After: timeout, Last: last}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollEvery):
		}
	}
}

// Job is a long operation the workbench is doing. Live: a handle to an id.
type Job struct {
	w  *Workbench
	id string
}

// Job makes a handle to one by id.
func (w *Workbench) Job(id string) Job { return Job{w: w, id: id} }

// Jobs is everything in flight.
func (w *Workbench) Jobs(ctx context.Context) ([]JobInfo, error) {
	snap, err := w.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	raw, _ := snap["jobs"].([]any)
	out := make([]JobInfo, 0, len(raw))
	for _, r := range raw {
		m, _ := r.(map[string]any)
		out = append(out, JobInfo{
			ID:       str(m["id"]),
			What:     str(m["what"]),
			Done:     num(m["done"]),
			Total:    num(m["total"]),
			Finished: m["finished"] == true,
		})
	}
	return out, nil
}

// Info is where this job has got to, or false when it is no longer listed -
// which means finished, because a job that has ended is removed.
func (j Job) Info(ctx context.Context) (JobInfo, bool, error) {
	all, err := j.w.Jobs(ctx)
	if err != nil {
		return JobInfo{}, false, err
	}
	for _, x := range all {
		if x.ID == j.id {
			return x, true, nil
		}
	}
	return JobInfo{}, false, nil
}

// Cancel stops it, where whoever started it left a way to.
//
// A job with no cancel refuses by name rather than silently doing nothing: an
// operator who asked deserves to be told, not left watching a bar that
// carries on.
func (j Job) Cancel(ctx context.Context) error {
	return j.w.Do(ctx, "job.cancel", map[string]any{"id": j.id})
}

// Wait blocks until it is gone from the list.
func (j Job) Wait(ctx context.Context, timeout time.Duration) error {
	return waitFor(ctx, timeout, "job "+j.id, func() (bool, string, error) {
		info, live, err := j.Info(ctx)
		if err != nil {
			return false, "", err
		}
		if !live || info.Finished {
			return true, "", nil
		}
		return false, fmt.Sprintf("%s, %d of %d", info.What, info.Done, info.Total), nil
	})
}

// WaitIdle waits for every job to finish - the honest way to wait out a warm,
// which is what most of them are.
func (w *Workbench) WaitIdle(ctx context.Context, timeout time.Duration) error {
	return waitFor(ctx, timeout, "the workbench to go idle", func() (bool, string, error) {
		jobs, err := w.Jobs(ctx)
		if err != nil {
			return false, "", err
		}
		if len(jobs) == 0 {
			return true, "", nil
		}
		return false, fmt.Sprintf("%d still running, first is %q",
			len(jobs), jobs[0].What), nil
	})
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) int {
	f, _ := v.(float64)
	return int(f)
}
