package session

import (
	"context"
	"testing"
	"time"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
)

// One cell, with everything it reports visible.
//
// Run through the socket, four cells came back as zeros with no error, which
// is the shape of a result rather than a failure. This says which step is not
// happening.
func TestOneExperimentCellReportsWhatItDid(t *testing.T) {
	if testing.Short() {
		t.Skip("starts real firmware")
	}
	store := state.New(10)
	sim := &Sim{}
	Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)
	if _, err := store.Do(ctx, "project.open", "../../fixtures/fixture-fife-strict.json"); err != nil {
		t.Skip("no fixture:", err)
	}

	e := sim.experiment()
	e.Senders = []string{"AngusOutlaw1"}
	e.RunForMs, e.SendAtMs = 40_000, 15_000

	began := time.Now()
	r := sim.runArm(context.Background(), e,
		ExpArm{Label: "v1.17", RepeaterVersion: "repeater-v1.17.0"}, 1, sim.nodes)
	took := time.Since(began)

	t.Logf("took %s", took.Round(time.Second))
	t.Logf("firmware attached: %d", r.Firmware)
	t.Logf("tx %d  rx %d  delivered %d  redundant %d  airtime %.0f ms",
		r.TX, r.RX, r.Delivered, r.Redundant, r.AirtimeMs)
	if r.Err != "" {
		t.Fatalf("cell failed: %s", r.Err)
	}
	// Ninety seconds of simulated time driving fifty-odd real processes cannot
	// finish in a couple of seconds. If it did, no firmware was attached.
	if took < 10*time.Second {
		t.Errorf("the cell finished in %s: nothing was actually run", took.Round(time.Second))
	}
	if r.TX == 0 {
		t.Errorf("no transmissions at all: the message was not originated")
	}
}
