package engine

import (
	"context"
	"testing"
	"time"
)

// A node stopped in a debugger is held, not failed, and the rest of the mesh
// goes on; when the debugger lets it go it answers, and the next tick takes it
// to the run's current instant (#191).
func TestANodeInADebuggerIsHeldAndRejoins(t *testing.T) {
	old := debugGrace
	debugGrace = 200 * time.Millisecond
	t.Cleanup(func() { debugGrace = old })

	e := New(flatGround{}, Config{StepMs: 10, FirmwareTickTimeout: 30 * time.Second})
	defer func() { _ = e.Close() }()
	stopped := attachedNode(t, e, "stopped")
	talker := attachedNode(t, e, "talker")
	e.SetDebug("stopped", &Debug{})

	step := func() {
		t.Helper()
		done := make(chan error, 1)
		go func() { done <- e.Step(context.Background()) }()
		talker.ack(t, talker.nextTick(t))
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Step: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("a node in a debugger stopped the run")
		}
	}

	// The first tick reaches it and goes unanswered: held, not down.
	step()
	heldAt := stopped.nextTick(t)
	if down := e.FirmwareFailures(); len(down) != 0 {
		t.Fatalf("a held node was marked down: %+v", down)
	}
	if at, ok := e.Held("stopped"); !ok || at != heldAt {
		t.Fatalf("Held = %d, %v; want %d, true", at, ok, heldAt)
	}

	// While held it is not sent another tick.
	step()
	select {
	case at := <-stopped.ticks:
		t.Fatalf("a held node was ticked again, to %d", at)
	case <-time.After(100 * time.Millisecond):
	}

	// Let go: it answers the tick it was held at, and rejoins at the run's
	// current instant.
	stopped.ack(t, heldAt)
	time.Sleep(50 * time.Millisecond)
	done := make(chan error, 1)
	go func() { done <- e.Step(context.Background()) }()
	at := stopped.nextTick(t)
	talker.ack(t, talker.nextTick(t))
	stopped.ack(t, at)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if at <= heldAt {
		t.Errorf("rejoined at %d, not past the %d it was held at", at, heldAt)
	}
	if _, ok := e.Held("stopped"); ok {
		t.Error("still held after answering")
	}

	// Not debugged, the same silence is a failure, as it always was.
	e.SetDebug("stopped", nil)
	if _, ok := e.Debugging("stopped"); ok {
		t.Error("SetDebug(nil) left it debugged")
	}
}
