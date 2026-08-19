package session

import (
	"context"
	"math"
	"os"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
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
	if _, err := store.Do(ctx, "project.open", "../../../fixtures/fixture-fife-strict.json"); err != nil {
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

// What the run-to-run noise actually is.
//
// Three readings of this, and the first two were wrong. Identical numbers
// across seeds looked like a broken seed; it is not, because with one
// originator and rx_delay_base at its shipped 0.0f every repeater relays
// exactly once and the count is a property of the topology - this project's
// own study measured 93 transmissions on each of eight seeds. Then two runs
// differed, which looked like the seed working; it is not that either. The
// cells are paced to the wall clock because real firmware boots and retries on
// it, so ordinary scheduling jitter decides which transmissions collide.
//
// So the noise is real, it is machine noise rather than seed noise, and the
// way to measure it is to run one arm twice and look. That is what this does.
// It asserts the size, not the sign: a runner whose repeats differ by a third
// cannot support any claim about a firmware change, and one whose repeats are
// bit-identical gives no floor to measure a claim against.
func TestTheNoiseFloorIsMeasurable(t *testing.T) {
	if testing.Short() {
		t.Skip("starts real firmware twice")
	}
	// The number this produces is a property of the machine, not of the code:
	// it measures how much run-to-run spread the host's own scheduling adds.
	// A shared two-core runner has far too much of it, so on CI the assertion
	// fails for a reason that says nothing about MeshBench - and a test that
	// is always red is a test everybody learns to ignore. It runs on a real
	// machine, where the answer means something.
	if os.Getenv("CI") != "" {
		t.Skip("measures the host's own scheduling noise; a shared CI runner has too much to measure through")
	}
	store := state.New(10)
	sim := &Sim{}
	Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)
	if _, err := store.Do(ctx, "project.open", "../../../fixtures/fixture-fife-strict.json"); err != nil {
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
	if a.TX == 0 || b.TX == 0 {
		t.Fatal("a cell measured nothing")
	}
	spread := math.Abs(float64(a.TX-b.TX)) / float64(a.TX) * 100
	t.Logf("seed 1: tx %d rx %d delivered %d", a.TX, a.RX, a.Delivered)
	t.Logf("seed 2: tx %d rx %d delivered %d", b.TX, b.RX, b.Delivered)
	t.Logf("run-to-run spread on transmissions: %.1f%%", spread)

	// Loose on purpose. The number worth having is the one printed above,
	// which is what any claim about a firmware difference has to clear; this
	// only fails when the runner has stopped being usable for comparison at
	// all.
	if spread > 25 {
		t.Errorf("repeats differ by %.1f%%: no firmware difference could be "+
			"distinguished from this", spread)
	}
}

// A region is spelled two ways and both are right: the repeater CLI takes the
// bare name, the key on the wire is derived from the "#" form. Sending under
// the bare name matches no repeater, and nothing reports an error at either end.
func TestAScopeIsCanonicalisedBeforeItsKeyIsDerived(t *testing.T) {
	for _, in := range []string{"sco", "#sco", "  sco  ", " #sco"} {
		if got := canonicalScope(in); got != "#sco" {
			t.Errorf("canonicalScope(%q) = %q, want %q", in, got, "#sco")
		}
	}
	// Empty stays empty: no scope asked for means send unscoped, which is a
	// choice, and is not the same as sending under "#".
	if got := canonicalScope("  "); got != "" {
		t.Errorf("canonicalScope(blank) = %q, want empty", got)
	}
}
