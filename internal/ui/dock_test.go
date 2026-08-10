package ui

import "testing"

// The pop-out contract, twice shipped broken and twice declared fixed: a
// panel must leave the main window and be able to come back.
//
// The mechanism is what is asserted, because the mistake was always in the
// mechanism. A *docked* window ignores SetNextWindowPos entirely — its dock
// node owns its geometry — so the original detach button moved nothing at
// all; undocking has to be a separate act, queued before Begin. The end-to-
// end proof is over the control socket, where imgui reports whether the panel
// really has its own platform window; this holds the plumbing that gets it
// there.
func TestPopOutAndDockBackAreQueuedSeparately(t *testing.T) {
	a := New(flatT{})

	a.popOut("Waterfall")
	if !a.detach["Waterfall"] {
		t.Fatal("pop out did not queue an undock")
	}
	if a.redock["Waterfall"] {
		t.Fatal("pop out also queued a dock-back")
	}

	a.dockBack("Waterfall")
	if !a.redock["Waterfall"] {
		t.Fatal("dock back did not queue")
	}

	// applyRedocks drains the queue. It cannot run here (there is no imgui
	// context in a test), but the queue's shape is what the frame loop reads,
	// and an entry left behind would re-dock the window on every later frame —
	// which would pin it into the main window for ever.
	if len(a.redock) != 1 {
		t.Fatalf("redock queue = %v", a.redock)
	}
}
