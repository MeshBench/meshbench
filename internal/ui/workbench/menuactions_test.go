package workbench

import (
	"context"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// Play with nothing pinned must ask which build each role should run, not
// refuse into the status bar. The pre-check once dispatched on a misspelt
// action name, so it never ran and every other check still passed.
func TestPlayWithUnpinnedFirmwareAsks(t *testing.T) {
	h := newShellHarness(t)
	st := state.New(10)
	started := make(chan struct{}, 1)
	st.Handle("firmware.needed", func(w *state.World, params any) (any, error) {
		return map[string]any{"roles": []any{map[string]any{
			"role": "simple_repeater", "nodes": 3,
			"choices": []string{"v1.2.0", "v1.3.0"},
		}}}, nil
	})
	st.Handle("sim.start", func(w *state.World, params any) (any, error) {
		started <- struct{}{}
		return nil, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	menuDeps{sh: h.sh, st: st, ctx: ctx}.onMenu("sim.start")

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		h.frame()
		if h.sh.Ask.Showing() {
			select {
			case <-started:
				t.Fatal("sim.start fired even though a role had nothing to run")
			default:
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("play with unpinned firmware never asked which build to run")
}

// And when every role has a build, play must go straight to the verb - the
// pre-check exists to ask a missing question, not to gate a ready mesh.
func TestPlayWithFirmwarePinnedStarts(t *testing.T) {
	h := newShellHarness(t)
	st := state.New(10)
	started := make(chan struct{}, 1)
	st.Handle("firmware.needed", func(w *state.World, params any) (any, error) {
		return map[string]any{"roles": []any{}}, nil
	})
	st.Handle("sim.start", func(w *state.World, params any) (any, error) {
		started <- struct{}{}
		return nil, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	menuDeps{sh: h.sh, st: st, ctx: ctx}.onMenu("sim.start")

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("play with every role pinned never reached sim.start")
	}
	if h.sh.Ask.Showing() {
		t.Fatal("a question was asked with nothing to ask about")
	}
}
