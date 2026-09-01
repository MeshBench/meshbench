package session

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// Two jobs, one of them over: what a script is told has to be about the one
// still running.
func TestAFinishedJobStopsCounting(t *testing.T) {
	st := state.New(10)
	s := &Sim{}
	Register(st, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	post := func(j state.Job) {
		t.Helper()
		if _, err := st.Do(ctx, "job.progress", j); err != nil {
			t.Fatalf("job.progress: %v", err)
		}
	}
	post(state.Job{ID: "fw-1", What: "downloading repeater 2.7.26",
		Done: 3400, Total: 6800})
	post(state.Job{ID: "tiles", What: "fetching terrain, 84 MB of about 366 MB",
		Done: 1420, Total: 6233})

	out, err := st.Do(ctx, "job.list", nil)
	if err != nil {
		t.Fatalf("job.list: %v", err)
	}
	m := out.(map[string]any)
	if m["running"] != 2 {
		t.Errorf("two jobs are running and job.list says %v", m["running"])
	}
	rows := m["jobs"].([]map[string]any)
	if len(rows) != 2 {
		t.Fatalf("job.list returned %d rows, not 2", len(rows))
	}
	if rows[0]["id"] != "fw-1" || rows[0]["what"] == "" || rows[0]["total"] != 6800 {
		t.Errorf("a row does not say what it is or how far it has got: %v", rows[0])
	}

	// Finished, and the file is on disk. The count used to stay at two for
	// the rest of the session, so nothing could tell whether it was safe to
	// carry on.
	post(state.Job{ID: "fw-1", What: "downloaded repeater 2.7.26",
		Done: 1, Total: 1, Finished: true})
	out, err = st.Do(ctx, "job.list", nil)
	if err != nil {
		t.Fatalf("job.list: %v", err)
	}
	m = out.(map[string]any)
	if m["running"] != 1 {
		t.Errorf("one job is running and job.list says %v", m["running"])
	}
	if rows := m["jobs"].([]map[string]any); len(rows) != 1 || rows[0]["id"] != "tiles" {
		t.Errorf("the finished job is still being offered as running: %v", rows)
	}
	// It is still reachable, because a caller that polls a moment late still
	// has to be able to learn how the thing it waited for turned out.
	out, err = st.Do(ctx, "job.list", map[string]any{"all": true})
	if err != nil {
		t.Fatalf("job.list all: %v", err)
	}
	if rows := out.(map[string]any)["jobs"].([]map[string]any); len(rows) != 2 {
		t.Errorf("a finished job cannot be looked up at all: %v", rows)
	}
}

// The one-line answer names the running job, and names it by id so a waiter
// does not have to match on prose.
func TestStatusNamesTheRunningJob(t *testing.T) {
	st := state.New(10)
	s := &Sim{}
	Register(st, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	if _, err := st.Do(ctx, "job.progress", state.Job{
		ID: "tiles", What: "fetching terrain, 84 MB of about 366 MB",
		Done: 1420, Total: 6233}); err != nil {
		t.Fatalf("job.progress: %v", err)
	}
	out, err := st.Do(ctx, "session.status", nil)
	if err != nil {
		t.Fatalf("session.status: %v", err)
	}
	m := out.(map[string]any)
	if m["jobs"] != 1 {
		t.Errorf("session.status says %v jobs are running, not 1", m["jobs"])
	}
	job, ok := m["job"].(map[string]any)
	if !ok {
		t.Fatal("session.status offered no running job at all")
	}
	if job["id"] != "tiles" || job["done"] != 1420 || job["total"] != 6233 {
		t.Errorf("session.status cannot say what is running or how far it has got: %v", job)
	}
}
