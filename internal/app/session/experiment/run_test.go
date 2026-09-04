package experiment

import (
	"context"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
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
	sim := &session.Sim{}
	session.Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)
	if _, err := store.Do(ctx, "project.open", "../../../fixtures/fixture-fife-strict.json"); err != nil {
		t.Skip("no fixture:", err)
	}

	e := matrixOf(sim)
	e.Senders = []string{"AngusOutlaw1"}
	e.RunForMs, e.SendAtMs = 40_000, 15_000

	began := time.Now()
	r := runArm(context.Background(), sim, e,
		session.ExpArm{Label: "v1.17", RepeaterVersion: "repeater-v1.17.0"}, 1, sim.Nodes())
	took := time.Since(began)

	t.Logf("took %s", took.Round(time.Second))
	t.Logf("firmware attached: %d", r.Firmware)
	t.Logf("tx %d  rx %d  delivered %d  redundant %d  airtime %.0f ms",
		r.TX, r.RX, r.Delivered, r.Redundant, r.AirtimeMs)
	if r.Err != "" {
		// A machine too old for the published build is not a result this test
		// can produce, and reporting it as an experiment fault is how it spent
		// a fortnight looking like a flake: the lab pool holds one runner that
		// pins MeshBench's own glibc floor at 2.35 deliberately, MeshCore's
		// builds want 2.38, and which runner a job lands on is a lottery.
		if strings.Contains(r.Err, firmware.HostTooOld) {
			t.Skip("this machine cannot run the published build: ", r.Err)
		}
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
//
// The number this produces is a property of the machine, not of the code: it
// measures how much run-to-run spread the HOST'S OWN SCHEDULING adds. That
// spread comes from load, and `go test ./...` itself creates load by running
// packages in parallel, so a gate on the CI env var alone left this red on an
// ordinary developer's own workstation for a reason that says nothing about
// MeshBench. It is opt-in instead of opt-out: nobody gets it by accident, and
// it has to be asked for, alone, on a machine that is otherwise quiet.
//
//	MESHBENCH_NOISE_FLOOR=1 go test ./internal/app/session/ -run TestTheNoiseFloorIsMeasurable -v
func TestTheNoiseFloorIsMeasurable(t *testing.T) {
	if testing.Short() {
		t.Skip("starts real firmware twice")
	}
	if os.Getenv("MESHBENCH_NOISE_FLOOR") == "" {
		t.Skip("measures the HOST's own scheduling noise, not MeshBench: run it deliberately, " +
			"alone and not under a parallel `go test ./...`, with MESHBENCH_NOISE_FLOOR=1")
	}
	// Opt-in does not make a shared runner quiet: it only proves somebody
	// meant to run this. A CI box still has too much load of its own to
	// measure through, so it stays excluded even when asked for.
	if os.Getenv("CI") != "" {
		t.Skip("measures the host's own scheduling noise; a shared CI runner has too much to measure through")
	}
	store := state.New(10)
	sim := &session.Sim{}
	session.Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)
	if _, err := store.Do(ctx, "project.open", "../../../fixtures/fixture-fife-strict.json"); err != nil {
		t.Skip("no fixture:", err)
	}

	e := matrixOf(sim)
	e.Senders = []string{"AngusOutlaw1"}
	e.RunForMs, e.SendAtMs = 45_000, 20_000
	arm := session.ExpArm{Label: "v1.17", RepeaterVersion: "repeater-v1.17.0"}

	a := runArm(context.Background(), sim, e, arm, 1, sim.Nodes())
	b := runArm(context.Background(), sim, e, arm, 2, sim.Nodes())
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
		if got := session.CanonicalScope(in); got != "#sco" {
			t.Errorf("session.CanonicalScope(%q) = %q, want %q", in, got, "#sco")
		}
	}
	// Empty stays empty: no scope asked for means send unscoped, which is a
	// choice, and is not the same as sending under "#".
	if got := session.CanonicalScope("  "); got != "" {
		t.Errorf("session.CanonicalScope(blank) = %q, want empty", got)
	}
}

// A control arm has to be identical to the arm it duplicates in everything
// except the variable under test, and it was not.
//
// The flooded message carried the arm's label, so two arms differing only in
// their name flooded different numbers of bytes. Airtime scales with payload
// and airtime is what collides, which makes the size of the message part of
// what a contention experiment is measuring - and nothing in a result could
// show that the arms had not been given the same one. The seed is in the text
// as well, so two runs of a single arm parted company at ten, where the number
// grows a digit, and the seed was not the only thing separating them.
func TestEveryCellOfAnExperimentFloodsTheSameSize(t *testing.T) {
	e := &experiment{
		Arms: []session.ExpArm{
			{Label: "control"},
			{Label: "cancel a queued relay"},
			{Label: "rx_delay_base 10.0 · loop strict"},
		},
		Seeds: []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	}
	want := len(e.cellText(e.Arms[0], e.Seeds[0]))
	for _, a := range e.Arms {
		for _, seed := range e.Seeds {
			if got := len(e.cellText(a, seed)); got != want {
				t.Errorf("%s at seed %d floods %d bytes where %s at seed %d "+
					"floods %d: the arms differ in airtime, which is part of "+
					"what they are being compared on",
					a.Label, seed, got, e.Arms[0].Label, e.Seeds[0], want)
			}
		}
	}

	// Still identifiable: a capture has to say which cell it came from.
	if !strings.HasPrefix(e.cellText(e.Arms[0], 3), "control seed 3") {
		t.Errorf("a cell no longer names itself: %q", e.cellText(e.Arms[0], 3))
	}

	// And the experiment's own size is a floor rather than a replacement for
	// it: padding shorter than the widest label leaves the arms uneven again.
	e.Bytes = 400
	if got := len(e.cellText(e.Arms[0], 1)); got != 400 {
		t.Errorf("padded to %d bytes, want the 400 the experiment asked for", got)
	}
	e.Bytes = 4
	if got := len(e.cellText(e.Arms[0], 1)); got != want {
		t.Errorf("a padding of 4 gave %d bytes, want the matrix's own %d - "+
			"the arms are uneven again", got, want)
	}
}
