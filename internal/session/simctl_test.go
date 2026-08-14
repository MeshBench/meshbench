package session

import (
	"context"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/gui/state"
)

// A bounded run has to stop on its own. If it does not, a script that starts
// one and polls for the end waits forever, which is the failure this verb
// exists to prevent.
func TestARunStopsWhereItSaidItWould(t *testing.T) {
	st := state.New(10)
	Register(st, &Sim{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	if _, err := st.Do(ctx, "sim.run", map[string]any{"for_ms": float64(200)}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		v, err := st.Do(ctx, "sim.state", nil)
		if err != nil {
			t.Fatal(err)
		}
		m := v.(map[string]any)
		if !m["playing"].(bool) {
			if got := m["now_ms"].(uint32); got < 200 {
				t.Fatalf("stopped early at %d ms, wanted at least 200", got)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the run never stopped; a poller would have waited forever")
}

// Every one of these is polled by something that has no window, so none of
// them may fail merely because nothing is loaded yet.
func TestSimControlIsSafeWithNoNetwork(t *testing.T) {
	st := state.New(10)
	Register(st, &Sim{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	if _, err := st.Do(ctx, "sim.state", nil); err != nil {
		t.Errorf("sim.state: %v", err)
	}
	if _, err := st.Do(ctx, "sim.speed", map[string]any{"factor": float64(2)}); err != nil {
		t.Errorf("sim.speed: %v", err)
	}
	// These two do need a network, and must say so rather than panic.
	if _, err := st.Do(ctx, "sim.reset", nil); err == nil {
		t.Error("sim.reset with no network should say so")
	}
}

func TestSpeedAcceptsBothSpellings(t *testing.T) {
	st := state.New(10)
	Register(st, &Sim{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	v, err := st.Do(ctx, "sim.speed", map[string]any{"step_ms": float64(40)})
	if err != nil {
		t.Fatal(err)
	}
	if got := v.(map[string]any)["step_ms"].(uint32); got != 40 {
		t.Errorf("step_ms: got %d, want 40", got)
	}
	v, err = st.Do(ctx, "sim.speed", map[string]any{"factor": float64(3)})
	if err != nil {
		t.Fatal(err)
	}
	if got := v.(map[string]any)["step_ms"].(uint32); got != 30 {
		t.Errorf("factor 3: got %d ms per tick, want 30", got)
	}
}
