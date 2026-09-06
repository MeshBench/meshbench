package workbench

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// A machine that is only waiting on an answer is not a machine that is broken.
//
// The check opened on needed+undecided and then said the blocking sentence for
// both, so a fully provisioned machine whose only outstanding row was the
// update-check question got the Setup page in front of the map on every
// launch, announcing "this machine is not set up yet" while the page it had
// just opened said "nothing is broken". Both sentences on screen together, and
// one of them wrong.
func TestAnUnansweredQuestionDoesNotOpenSetup(t *testing.T) {
	opened, said := runFirstRunCheck(t, map[string]any{"needed": 0, "undecided": 1})

	if opened {
		t.Error("Setup opened for a machine with nothing blocking, which is the " +
			"splash screen firstrun.go exists to avoid")
	}
	if !strings.Contains(said, "nothing is broken") {
		t.Errorf("the question should still be mentioned, in the page's own "+
			"words; got %q", said)
	}
	if strings.Contains(said, "not set up yet") {
		t.Errorf("nothing is blocking, so this must not claim otherwise: %q", said)
	}
}

// Something that cannot run still opens the page and still says so.
func TestSomethingBlockingStillOpensSetup(t *testing.T) {
	opened, said := runFirstRunCheck(t, map[string]any{"needed": 2, "undecided": 1})

	if !opened {
		t.Error("a blocking row did not open Setup")
	}
	if !strings.Contains(said, "not set up yet") {
		t.Errorf("a blocking row should say so; got %q", said)
	}
}

// A machine with nothing outstanding sees nothing at all.
func TestAReadyMachineIsLeftAlone(t *testing.T) {
	opened, said := runFirstRunCheck(t, map[string]any{"needed": 0, "undecided": 0})

	if opened {
		t.Error("Setup opened on a machine that wants nothing")
	}
	if said != "" {
		t.Errorf("a ready machine should be told nothing; got %q", said)
	}
}

// runFirstRunCheck drives the real check against a stubbed setup.check and
// reports what it opened and what it said.
func runFirstRunCheck(t *testing.T, check map[string]any) (opened bool, said string) {
	t.Helper()
	st := state.New(10)
	var mu sync.Mutex
	st.Handle("setup.check", func(*state.World, any) (any, error) { return check, nil })
	st.Handle("panel.open", func(_ *state.World, p any) (any, error) {
		mu.Lock()
		defer mu.Unlock()
		opened = true
		return nil, nil
	})
	st.Handle("ui.said", func(_ *state.World, p any) (any, error) {
		mu.Lock()
		defer mu.Unlock()
		said, _ = p.(string)
		return nil, nil
	})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go st.Run(ctx)
	openSetupIfNotReady(ctx, st)

	// The check waits firstRunDelay for a window to exist before it asks.
	deadline := time.Now().Add(firstRunDelay + 5*time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := opened || said != ""
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	// A ready machine says nothing, so give it the whole delay before deciding
	// that silence was the answer.
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	return opened, said
}
