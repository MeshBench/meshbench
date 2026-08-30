package control

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// Replies come only from Pump, on the frame thread; anything that stops that
// loop stops every reply. CallContext exists so a caller waiting on one is
// not waiting for ever - it is what tools/soak and the CI regression scripts
// need, since a hang there is worse than an error.

func TestCallContextReturnsWhenTheServerNeverPumps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wedged.sock")
	// No Pump loop at all: whatever is queued is never answered, which is
	// what a wedged frame thread looks like from a client's side.
	srv, err := ListenAt(path, func(string, json.RawMessage) (any, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	c, err := DialAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = c.CallContext(ctx, "who", nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("a call against a workbench that never pumps returned success")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("the call took %s to give up on a 200ms deadline", elapsed)
	}

	// The decoder's place in the stream cannot be trusted after a call ended
	// by its context rather than by a reply - the answer to that first call
	// may still arrive - so a second call must fail fast rather than risk
	// reading it as its own.
	if _, err := c.Call("who", nil); err == nil {
		t.Fatal("a client used after a cancelled call returned success")
	}
}

// Call itself keeps its old, deadline-free behaviour: it is CallContext with
// context.Background(), and a workbench that answers promptly still answers
// promptly.
func TestCallStillWorksWithNoDeadline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plain.sock")
	serveAt(t, path)

	c, err := DialAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	if _, err := c.Call("who", nil); err != nil {
		t.Fatalf("a plain call against a working server failed: %v", err)
	}
}
