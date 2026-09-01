package session

import (
	"context"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// Opening a network stops the run that was going.
//
// The clock and the engine are not independent: the tick steps whichever
// engine is live, and opening a network replaces that engine and stops the
// firmware processes behind it. Left playing, the next tick after an open
// steps a network the operator never started, while an attach may still be
// landing in it - which is how replacing a network under a running clock
// stopped the whole application rather than one node.
//
// It says so, because a run that stops on its own without saying why is the
// same silence.
func TestOpeningANetworkStopsTheRunThatWasGoing(t *testing.T) {
	st := state.New(10)
	sm := &Sim{}
	Register(st, sm)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	if _, err := st.Do(ctx, "project.open",
		"../../../fixtures/fixture-fife-strict.json"); err != nil {
		t.Skip("no fixture:", err)
	}
	if _, err := st.Do(ctx, "sim.run", map[string]any{"for_ms": 60_000}); err != nil {
		t.Fatal(err)
	}
	if snap := st.Snapshot(); !snap.Playing {
		t.Fatal("the run did not start, so this test would prove nothing")
	}

	if _, err := st.Do(ctx, "project.open",
		"../../../fixtures/fixture-fife-permissive.json"); err != nil {
		t.Fatal(err)
	}
	snap := st.Snapshot()
	if snap.Playing {
		t.Error("the run carried on over a network that had been replaced")
	}
	if snap.RunUntilMs != 0 {
		t.Errorf("the old run's finish time survived the open: %d ms", snap.RunUntilMs)
	}
	said := strings.Join(snap.FullLog, "\n")
	if !strings.Contains(said, "paused") {
		t.Errorf("nothing said the run had been stopped; the log was:\n%s", said)
	}
}
