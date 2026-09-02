package session

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// Is a verb still reading its node when something else is passed beside it?
//
// It was not. soleString answers with the only value of a single-key object and
// with nothing at all when there are two, so a caller who passed the documented
// key and one thing more - a region, a tab, a mode - handed these verbs an empty
// name. Every one of them then refused, naming `""`, which reads as a network
// with a node missing rather than as a parameter that was never looked up.
//
// The refusal is the assertion rather than the success: node.energy wants a
// solar year, node.stop wants a running node and coverage.compute wants terrain,
// and none of those is a headless two-node session. What none of them may do is
// refuse a name that was given.
func TestTheDocumentedKeyIsReadWhenSomethingElseIsBesideIt(t *testing.T) {
	t.Setenv("MESHBENCH_ENERGY", "1")
	store, ctx := openedFixture(t)
	name := store.Snapshot().Nodes[0].Name

	for _, tc := range []struct {
		verb   string
		params map[string]any
	}{
		{"node.energy", map[string]any{"node": name, "region": "tay"}},
		{"node.stop", map[string]any{"node": name, "why": "testing"}},
		{"node.start", map[string]any{"node": name, "why": "testing"}},
		{"coverage.compute", map[string]any{"node": name, "cells": 800}},
		{"nodes.select", map[string]any{"node": name, "add": false}},
		{"map.centre", map[string]any{"node": name, "zoom": 9}},
	} {
		_, err := store.Do(ctx, tc.verb, tc.params)
		if err != nil && strings.Contains(err.Error(), `""`) {
			t.Errorf("%s given %q alongside another key answered %v; the name "+
				"was passed under the key the description asks for",
				tc.verb, name, err)
		}
	}

	// And one whose success is visible rather than only its refusal. The verbs
	// above answer with what the session can do about them; this one moves the
	// cursor, and with a second key beside the name it used to move it to
	// nothing at all and report that as a selection.
	second := store.Snapshot().Nodes[1].Name
	if _, err := store.Do(ctx, "nodes.select",
		map[string]any{"node": second, "add": false}); err != nil {
		t.Fatal(err)
	}
	if got := selectedInStore(store); got != second {
		t.Errorf("nodes.select with a second key beside the name selected %q, "+
			"want %q", got, second)
	}
}

// A fetch it cannot address is refused, not started.
//
// import.describe read its url and, when it could not, fetched the empty string
// on a worker with a ninety second timeout and answered `{"url": ""}` - the
// success shape - while the failure went to import.failed, which the caller who
// asked never subscribes to. The refusal belongs to the call that was made.
func TestImportDescribeRefusesAFetchItCannotAddress(t *testing.T) {
	store, ctx := openedFixture(t)
	for _, params := range []any{
		map[string]any{"hours": 24},
		map[string]any{"url": 7},
		map[string]any{},
	} {
		got, err := store.Do(ctx, "import.describe", params)
		if err == nil {
			t.Errorf("import.describe %v answered %v; there was no url in it",
				params, got)
		}
	}
}

// And the single-key object the old socket's callers write still works.
func TestTheBareFormStillReaches(t *testing.T) {
	store, ctx := openedFixture(t)
	name := store.Snapshot().Nodes[1].Name
	if _, err := store.Do(ctx, "nodes.select", name); err != nil {
		t.Fatal(err)
	}
	if got := selectedInStore(store); got != name {
		t.Errorf("a bare name selected %q, want %q", got, name)
	}
	if _, err := store.Do(ctx, "nodes.select",
		map[string]any{"anything": name}); err != nil {
		t.Fatal(err)
	}
	if got := selectedInStore(store); got != name {
		t.Errorf("a single-key object selected %q, want %q", got, name)
	}
}

func selectedInStore(store *state.Store) string {
	for _, n := range store.Snapshot().Nodes {
		if n.Selected {
			return n.Name
		}
	}
	return ""
}
