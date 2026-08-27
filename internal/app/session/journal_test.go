package session_test

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// The journal records the commands that drove the world and leaves out the
// polls and the workers' own reports - which is what makes it a history rather
// than a log. Driven through the store, exactly as a client would.
func TestJournalRecordsCommandsNotPolls(t *testing.T) {
	store := state.New(10)
	sim := &session.Sim{}
	session.Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)

	// Two real commands, and two things that must not appear: a poll and a
	// worker callback.
	must := func(verb string, p any) {
		if _, err := store.Do(ctx, verb, p); err != nil {
			t.Fatalf("%s: %v", verb, err)
		}
	}
	// A command that needs no network, so the test is about the journal, not
	// the verbs: a stand-in the store records like any other.
	store.Handle("test.command", func(w *state.World, p any) (any, error) { return nil, nil })
	must("test.command", map[string]any{"seed": float64(42), "node": "GB7XYZ"})
	must("sim.state", nil)                                        // a poll
	must("job.progress", state.Job{ID: "x", What: "y", Total: 1}) // a worker callback

	got, err := store.Do(ctx, "session.journal", nil)
	if err != nil {
		t.Fatalf("session.journal: %v", err)
	}
	m := got.(map[string]any)
	if m["started_ms"].(int64) == 0 {
		t.Error("journal has no start time")
	}
	entries := m["entries"].([]state.JournalEntry)
	if len(entries) == 0 || entries[0].Verb != "session.start" {
		t.Fatalf("first journal entry is %v, want session.start", entries)
	}
	seen := map[string]string{}
	for _, e := range entries {
		seen[e.Verb] = e.Arg
	}
	if _, ok := seen["test.command"]; !ok {
		t.Error("journal did not record the command")
	}
	if seen["test.command"] != "node=GB7XYZ seed=42" {
		t.Errorf("command arg = %q, want node=GB7XYZ seed=42", seen["test.command"])
	}
	if _, ok := seen["sim.state"]; ok {
		t.Error("journal recorded the poll sim.state")
	}
	if _, ok := seen["job.progress"]; ok {
		t.Error("journal recorded the worker callback job.progress")
	}
	if _, ok := seen["session.journal"]; ok {
		t.Error("reading the journal wrote to it")
	}
}
