package session

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/gui/state"
)

// register builds a store with every verb on it, as main does.
func register(t *testing.T) (*state.Store, *Sim) {
	t.Helper()
	st := state.New(10)
	s := &Sim{}
	Register(st, s)
	return st, s
}

// The parity test the plan asks for (12.9), generated from what is registered
// rather than from a list in a document.
//
// A hand-counted list was already wrong by three when this was written the
// first time. Generating it means the test cannot pass while the document is
// stale, because there is no document.
func TestEveryVerbIsNamedAndReachable(t *testing.T) {
	st, _ := register(t)
	verbs := st.Verbs()
	if len(verbs) < 20 {
		t.Fatalf("only %d verbs registered; the interface has more routes than that", len(verbs))
	}
	seen := map[string]bool{}
	for _, v := range verbs {
		if seen[v] {
			t.Fatalf("%q is registered twice, so one of them is unreachable", v)
		}
		seen[v] = true
		if !strings.Contains(v, ".") {
			// Every verb is noun.verb, so a script reads as a sentence and
			// the namespace says which subsystem answers.
			t.Errorf("%q is not namespaced", v)
		}
		if strings.ToLower(v) != v {
			t.Errorf("%q is not lower case; a verb typed at a socket should not need a shift key", v)
		}
	}
}

// A verb that does not exist must say so by name. An unknown verb that returns
// a generic error is a typo somebody spends ten minutes on.
func TestAnUnknownVerbNamesItself(t *testing.T) {
	st, _ := register(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	_, err := st.Do(ctx, "nodes.selct", nil)
	if err == nil {
		t.Fatal("a misspelled verb succeeded")
	}
	if !strings.Contains(err.Error(), "nodes.selct") {
		t.Fatalf("the error does not name the verb: %v", err)
	}
}

// Verbs that change the world must be callable with no simulation Loaded, and
// must fail with a reason rather than a panic. The interface starts empty, and
// every one of these is reachable from a menu before anything is open.
func TestVerbsAreSafeBeforeAnythingIsLoaded(t *testing.T) {
	st, _ := register(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	verbs := append([]string(nil), st.Verbs()...)
	sort.Strings(verbs)
	for _, v := range verbs {
		if v == "project.open" {
			continue // needs a path, and has its own test
		}
		// A panic here fails the test by crashing it, which is the point:
		// these run on the store's goroutine and a panic takes the world with
		// it.
		_, _ = st.Do(ctx, v, nil)
	}
}
