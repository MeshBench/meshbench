// What a wait concluded, as opposed to merely that it stopped.
//
// These need no node and no emulator: the distinction a phase reads is decided
// in ordinary Go, and the point of each test below is which of the three
// outcomes came back, not what any board did.
package boardcheck

import (
	"context"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/sim/engine"
)

func quietEngine() *engine.Engine {
	return engine.New(flatEarth{}, engine.Config{StepMs: 10})
}

// A budget that genuinely runs out, with ctx healthy throughout, reports
// eventTimedOut rather than merely "not matched". The callers in probe.go tell
// a board's real failure from a cut-off probe by this alone.
func TestWaitForEventTimesOutHonestly(t *testing.T) {
	e := quietEngine()
	defer func() { _ = e.Close() }()

	atMs, outcome := waitForEvent(context.Background(), e, 100,
		func(engine.Event) bool { return false })
	if outcome != eventTimedOut {
		t.Fatalf("got outcome %v, want eventTimedOut", outcome)
	}
	if atMs != 0 {
		t.Errorf("got atMs %d on a timeout, want 0", atMs)
	}
}

// A wait cut short by ctx reports eventCancelled, distinct from eventTimedOut.
// This is the whole point: a probe truncated by its caller must never read the
// same as a board that had its full time and still did not do the thing.
func TestWaitForEventReportsCancellation(t *testing.T) {
	e := quietEngine()
	defer func() { _ = e.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already gone before the wait starts

	atMs, outcome := waitForEvent(ctx, e, advertBudgetMs,
		func(engine.Event) bool { return false })
	if outcome != eventCancelled {
		t.Fatalf("got outcome %v, want eventCancelled", outcome)
	}
	if atMs != 0 {
		t.Errorf("got atMs %d on a cancellation, want 0", atMs)
	}
}

// waitUntilQuiet surfaces the same distinction: a board still transmitting when
// quietMs and budgetMs both play out fairly is not the same finding as a probe
// that never got the chance to look.
func TestWaitUntilQuietReportsCancellation(t *testing.T) {
	e := quietEngine()
	defer func() { _ = e.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	quiet, cancelled := waitUntilQuiet(ctx, e, "bc-under-test", 10_000, advertBudgetMs)
	if quiet {
		t.Error("a cancelled wait reported quiet=true")
	}
	if !cancelled {
		t.Error("a cancelled wait did not report cancelled=true")
	}
}

// A node that never transmits reads as quiet without ctx coming into it, which
// is what makes quiet=false meaningful: it can only come from a genuine timeout
// or a real cancellation, never from a wait that simply ran normally.
func TestWaitUntilQuietPassesWhenNothingHasTransmitted(t *testing.T) {
	e := quietEngine()
	defer func() { _ = e.Close() }()

	quiet, cancelled := waitUntilQuiet(context.Background(), e, "bc-under-test", 100, advertBudgetMs)
	if !quiet {
		t.Error("a node that never transmitted was not reported quiet")
	}
	if cancelled {
		t.Error("an uncancelled wait reported cancelled=true")
	}
}

// ProbeBudget comes from the same numbers Probe waits on rather than a second
// figure written out by hand, so the two cannot drift apart the way
// board.probe's hardcoded five minutes had from advertBudgetMs.
func TestProbeBudgetIsDerivedFromThePhaseBudgets(t *testing.T) {
	want := time.Duration(probeWaitPhases)*time.Duration(advertBudgetMs)*time.Millisecond +
		probeFixedOverhead
	if got := ProbeBudget(); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

// The budget a caller must grant against what the phases can actually spend.
//
// Arithmetic rather than a run, because running it is twenty minutes. The
// numbers are the point: five waits of advertBudgetMs plus the settling and
// idle steps. board.probe used to grant a flat five minutes, which the first
// wait alone very nearly exhausts, so every phase after it was decided by the
// caller's deadline rather than by the board. ProbeBudget has to cover them.
func TestProbeBudgetOutlastsWhatThePhasesCanSpend(t *testing.T) {
	phases := time.Duration(probeWaitPhases) * time.Duration(advertBudgetMs) * time.Millisecond
	if ProbeBudget() < phases {
		t.Fatalf("ProbeBudget is %s, shorter than the %d waits it must cover (%s)",
			ProbeBudget(), probeWaitPhases, phases)
	}
	// The figure that used to be granted. Kept as evidence of the size of the
	// gap, so a future shrink of ProbeBudget back towards it fails here.
	const wasGranted = 5 * time.Minute
	if ProbeBudget() <= wasGranted {
		t.Errorf("ProbeBudget is %s, no better than the %s that truncated probes",
			ProbeBudget(), wasGranted)
	}
}
