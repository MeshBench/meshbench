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

// What varies with the seed, and what does not.
//
// I had this backwards. Transmissions being identical across seeds is not a
// broken seed: it is the documented behaviour of this firmware. With one
// originator and rx_delay_base at its shipped 0.0f, every repeater that hears
// the flood relays exactly once and markSeen() suppresses the second copy, so
// the count is a property of the topology. The project's own study measured
// 93 transmissions on each of eight seeds - not a mean of 93, the number 93,
// eight times.
//
// So this asserts the property that is real - a run repeats exactly, which is
// what makes an A/B comparison worth anything - and records the consequence
// that follows from it: the seed cannot be used to estimate a noise floor
// here, so results has to say so rather than present a spread of zero as
// though it were a measured one.
func TestARunRepeatsExactly(t *testing.T) {
	if testing.Short() {
		t.Skip("starts real firmware twice")
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
	e.RunForMs, e.SendAtMs = 45_000, 20_000
	arm := ExpArm{Label: "v1.17", RepeaterVersion: "repeater-v1.17.0"}

	a := sim.runArm(context.Background(), e, arm, 1, sim.nodes)
	b := sim.runArm(context.Background(), e, arm, 2, sim.nodes)
	if a.Err != "" || b.Err != "" {
		t.Fatalf("cells failed: %q / %q", a.Err, b.Err)
	}
	t.Logf("seed 1: tx %d rx %d delivered %d", a.TX, a.RX, a.Delivered)
	t.Logf("seed 2: tx %d rx %d delivered %d", b.TX, b.RX, b.Delivered)

	if a.TX == 0 || b.TX == 0 {
		t.Fatal("a cell measured nothing")
	}
	// Reproducible, which is the property an A/B comparison rests on: if the
	// same arm gave a different answer each time, no difference between arms
	// could be attributed to the arm.
	if a.TX != b.TX {
		t.Errorf("the same arm transmitted %d then %d: this run is not reproducible",
			a.TX, b.TX)
	}
	// And the consequence, asserted rather than assumed: with no spread from
	// the seed, the machinery must refuse to call the numbers a result.
	e.results = []ExpResult{
		{Arm: arm.Label, Seed: 1, TX: a.TX, RX: a.RX},
		{Arm: arm.Label, Seed: 2, TX: b.TX, RX: b.RX},
	}
	e.Arms = []ExpArm{arm, {Label: "other"}}
	if w := e.notAResultYet(); w == "" {
		t.Error("a zero spread was presented as a result")
	} else {
		t.Logf("reported as: %s", w)
	}
}
