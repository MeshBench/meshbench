package workbench

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// Deleting asks first, and only deletes what was asked about.
//
// The confirmation is the whole of this control: nodes.delete rebuilds the
// scenario and cannot be undone, and a map where a stray keypress removes a
// node is worse than a map with no delete at all.

// deletedNodes records what reached the store. Guarded because the handler
// runs on the store's goroutine and the assertions run on the test's.
type deletedNodes struct {
	mu    sync.Mutex
	names []string
}

func (d *deletedNodes) add(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.names = append(d.names, name)
}

func (d *deletedNodes) all() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.names...)
}

// deleteHarness is a store with nodes.delete recorded rather than performed.
func deleteHarness(t *testing.T) (callbacks, *deletedNodes) {
	t.Helper()
	st := state.New(10)
	deleted := &deletedNodes{}
	st.Handle("nodes.delete", func(_ *state.World, p any) (any, error) {
		m, _ := p.(map[string]any)
		name, _ := m["node"].(string)
		deleted.add(name)
		return nil, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	go st.Run(ctx)
	t.Cleanup(cancel)
	return callbacks{st: st, ctx: ctx}, deleted
}

// settle gives the worker deleteNodes starts time to reach the store.
func settle() { time.Sleep(200 * time.Millisecond) }

func TestDeletingAsksBeforeItRemovesAnything(t *testing.T) {
	c, deleted := deleteHarness(t)
	asked := ""
	c.chooser = func(title string, _ []string, _ func(string)) { asked = title }

	c.deleteNodes([]string{"Bishop Hill"})
	settle()
	if asked == "" {
		t.Fatal("deleting a node did not ask first")
	}
	if got := deleted.all(); len(got) != 0 {
		t.Fatalf("the question was still open and %v had already gone", got)
	}
}

func TestKeepingLeavesTheNodeAlone(t *testing.T) {
	c, deleted := deleteHarness(t)
	c.chooser = func(_ string, _ []string, pick func(string)) { pick("Keep") }

	c.deleteNodes([]string{"Bishop Hill"})
	settle()
	if got := deleted.all(); len(got) != 0 {
		t.Fatalf("answering Keep deleted %v", got)
	}
}

func TestConfirmingDeletesEveryNodeItAskedAbout(t *testing.T) {
	c, deleted := deleteHarness(t)
	c.chooser = func(_ string, _ []string, pick func(string)) { pick("Delete") }

	want := []string{"Bishop Hill", "Abernethy Repeater"}
	c.deleteNodes(want)
	settle()
	got := deleted.all()
	if len(got) != len(want) {
		t.Fatalf("deleted %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("deleted %v, want %v", got, want)
		}
	}
}

// One question for a whole selection, not one per node. Five prompts in a row
// is not a confirmation, it is an obstacle, and people learn to dismiss it.
func TestASelectionIsOneQuestion(t *testing.T) {
	c, _ := deleteHarness(t)
	asks := 0
	c.chooser = func(_ string, _ []string, _ func(string)) { asks++ }

	c.deleteNodes([]string{"A", "B", "C"})
	settle()
	if asks != 1 {
		t.Fatalf("deleting three nodes asked %d questions, want 1", asks)
	}
}
