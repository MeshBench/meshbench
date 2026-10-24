package session

import (
	"context"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// openedFife is a session on the Fife fixture with no firmware running, so a
// verb that reaches a node fails and says so - which is how these tests see
// the instant it ran.
func openedFife(t *testing.T) (*state.Store, context.Context, string) {
	t.Helper()
	st := state.New(10)
	Register(st, &Sim{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go st.Run(ctx)
	if _, err := st.Do(ctx, "project.open", "../../../fixtures/fixture-fife-strict.json"); err != nil {
		t.Skip("no fixture:", err)
	}
	return st, ctx, st.Snapshot().Nodes[0].Name
}

func do(t *testing.T, st *state.Store, ctx context.Context, verb string, p any) any {
	t.Helper()
	v, err := st.Do(ctx, verb, p)
	if err != nil {
		t.Fatalf("%s: %v", verb, err)
	}
	return v
}

func logCount(st *state.Store, s string) int {
	return strings.Count(strings.Join(st.Snapshot().FullLog, "\n"), s)
}

// A call given at_ms runs when the engine reaches that instant, and not when
// it arrived.
func TestHeldInputRunsAtItsInstantNotBefore(t *testing.T) {
	st, ctx, node := openedFife(t)
	got := do(t, st, ctx, "console.type", map[string]any{"node": node, "command": "advert", "at_ms": 500})
	if q, _ := got.(map[string]any)["queued"].(bool); !q {
		t.Fatalf("console.type with at_ms was not queued: %v", got)
	}
	const ran = "console.type at 0.5 s failed"
	do(t, st, ctx, "sim.settle", map[string]any{"steps": 40})
	if n := logCount(st, ran); n != 0 {
		t.Fatalf("ran at 400 ms, before its instant")
	}
	do(t, st, ctx, "sim.settle", map[string]any{"steps": 20})
	if n := logCount(st, ran); n != 1 {
		t.Fatalf("ran %d times by 600 ms, want once; log:\n%s", n, strings.Join(st.Snapshot().FullLog, "\n"))
	}
}

// An instant already passed is refused: "now" on a playing clock is exactly
// the moment nobody can promise.
func TestHeldInputInThePastIsRefused(t *testing.T) {
	st, ctx, node := openedFife(t)
	do(t, st, ctx, "sim.settle", map[string]any{"steps": 10})
	if _, err := st.Do(ctx, "console.type", map[string]any{"node": node, "command": "advert", "at_ms": 100}); err == nil {
		t.Fatal("at_ms at the current instant was accepted")
	}
}

// A node that is not there is refused when the call is made, not when it
// comes due, where the only trace would be a line in the log.
func TestHeldInputForAMissingNodeIsRefusedAtOnce(t *testing.T) {
	st, ctx, _ := openedFife(t)
	if _, err := st.Do(ctx, "console.type", map[string]any{"node": "Nobody", "command": "advert", "at_ms": 500}); err == nil {
		t.Fatal("a held call for a node that does not exist was queued")
	}
}

// Input that arrives on the wall clock while the clock plays makes the run
// unrepeatable, and the verb a script polls says so; a reset forgets it.
func TestInputWhilePlayingIsSaid(t *testing.T) {
	st, ctx, node := openedFife(t)
	do(t, st, ctx, "sim.run", map[string]any{"for_ms": 60_000})
	_, _ = st.Do(ctx, "console.type", map[string]any{"node": node, "command": "advert"})
	got := do(t, st, ctx, "sim.state", nil).(map[string]any)
	if ok, _ := got["reproducible"].(bool); ok {
		t.Fatal("typing at a playing clock left the run reproducible")
	}
	if why, _ := got["not_reproducible_why"].(string); !strings.Contains(why, "at_ms") {
		t.Errorf("the reason does not say how to avoid it: %q", why)
	}
	do(t, st, ctx, "sim.reset", nil)
	got = do(t, st, ctx, "sim.state", nil).(map[string]any)
	if ok, _ := got["reproducible"].(bool); !ok {
		t.Errorf("a reset run inherited the last one's reason: %v", got["not_reproducible_why"])
	}
}

// Held calls at one instant run in the order they were made.
func TestHeldInputKeepsItsOrder(t *testing.T) {
	var q inputQueue
	var order []string
	h := func(name string) state.Handler {
		return func(*state.World, any) (any, error) { order = append(order, name); return nil, nil }
	}
	q.add(200, "v", nil, h("c"))
	q.add(100, "v", nil, h("a"))
	q.add(100, "v", nil, h("b"))
	for _, in := range q.due(150) {
		_, _ = in.run(nil, nil)
	}
	for _, in := range q.due(200) {
		_, _ = in.run(nil, nil)
	}
	if got := strings.Join(order, ""); got != "abc" {
		t.Errorf("ran in order %q, want abc", got)
	}
	if len(q.due(1<<30)) != 0 {
		t.Error("something ran twice")
	}
}

// Adding a send mid-run does not repeat the ones already said.
func TestAddingASendDoesNotRepeatTheOthers(t *testing.T) {
	st, ctx, node := openedFife(t)
	do(t, st, ctx, "schedule.clear", nil)
	do(t, st, ctx, "schedule.add", map[string]any{"node": node, "at_ms": 100, "command": "advert"})
	do(t, st, ctx, "sim.settle", map[string]any{"steps": 20})
	failed := "scheduled send from " + node + " failed"
	if n := logCount(st, failed); n != 1 {
		t.Fatalf("the send went %d times, want once", n)
	}
	do(t, st, ctx, "schedule.add", map[string]any{"node": node, "at_ms": 5_000, "command": "advert"})
	do(t, st, ctx, "sim.settle", map[string]any{"steps": 20})
	if n := logCount(st, failed); n != 1 {
		t.Errorf("adding a later send repeated the first: %d times", n)
	}
}

// And a reset runs the schedule again: the second run of a fixture is a run.
func TestAResetSaysTheScheduleAgain(t *testing.T) {
	st, ctx, node := openedFife(t)
	do(t, st, ctx, "schedule.clear", nil)
	do(t, st, ctx, "schedule.add", map[string]any{"node": node, "at_ms": 100, "command": "advert"})
	do(t, st, ctx, "sim.settle", map[string]any{"steps": 20})
	do(t, st, ctx, "sim.reset", nil)
	do(t, st, ctx, "sim.settle", map[string]any{"steps": 20})
	if n := logCount(st, "scheduled send from "+node+" failed"); n != 2 {
		t.Errorf("over two runs the send went %d times, want twice", n)
	}
}

// One step moves the clock one step.
func TestOneStepMovesTheClock(t *testing.T) {
	st, ctx, _ := openedFife(t)
	got := do(t, st, ctx, "sim.step", nil).(map[string]any)
	if now, _ := got["now_ms"].(uint32); now != 10 {
		t.Errorf("one step left the clock at %d ms, want 10", now)
	}
}
