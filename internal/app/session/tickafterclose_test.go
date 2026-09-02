package session

import (
	"context"
	"testing"
)

// A tick must not step an engine that has been closed.
//
// w.Tick is installed with the body and never cleared, while Close takes the
// engine away - and from a different goroutine, which is what makes the window
// intermittent rather than reliable. sim.step calls the tick whenever one is
// installed, playing or not, so closing a network and pressing step is enough.
// The panic lands on the store's goroutine and takes the world with it, which
// from outside is the application disappearing.
func TestAStepAfterACloseDoesNotPanic(t *testing.T) {
	st, s := register(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	if _, err := st.Do(ctx, "project.new", map[string]any{"place": "Fife"}); err != nil {
		t.Fatalf("project.new: %v", err)
	}
	s.Close()
	if _, err := st.Do(ctx, "sim.step", nil); err != nil {
		t.Fatalf("sim.step after a close: %v", err)
	}
}
