package session

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/control"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// What a client is talking to, asked before anything else.
//
// A connection could not previously find out, so a client older or newer than
// the build failed halfway through a script rather than at the door - which in
// a CI run reads as a firmware regression rather than as a version mismatch.

func socketFor(t *testing.T) *control.Client {
	t.Helper()
	st := state.New(10)
	st.Handle("only.verb", func(*state.World, any) (any, error) { return nil, nil })
	ctx, cancel := context.WithCancel(context.Background())
	go st.Run(ctx)
	t.Cleanup(cancel)

	path := filepath.Join(t.TempDir(), "hello.sock")
	srv, err := ServeControlAt(ctx, st, path)
	if err != nil {
		t.Fatalf("serve: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	c, err := control.DialAt(path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestHelloSaysWhatItIs(t *testing.T) {
	c := socketFor(t)
	raw, err := c.Call("session.hello", nil)
	if err != nil {
		t.Fatalf("session.hello: %v", err)
	}
	var h Hello
	if err := json.Unmarshal(raw, &h); err != nil {
		t.Fatal(err)
	}
	if h.Protocol != control.Protocol {
		t.Errorf("protocol %d, want %d", h.Protocol, control.Protocol)
	}
	if h.Version == "" {
		t.Error("no version, so a client cannot say what it disagreed with")
	}
	if h.Mode == "" {
		t.Error("no mode, so a script cannot tell whether it has a window")
	}
	if h.Socket == "" {
		t.Error("no socket path, and there is no longer only one it could be")
	}
	if h.Verbs == 0 {
		t.Error("no verb count")
	}
	if h.PID == 0 || h.StartedAt.IsZero() {
		t.Error("nothing that tells a restart from a reconnect")
	}
	if time.Since(h.StartedAt) > time.Hour {
		t.Errorf("started_at is %v, which is not this run", h.StartedAt)
	}
}

// The path in hello is the path answered on, not the default. A client that
// reconnects by what hello told it has to reach the same workbench.
func TestHelloReportsTheSocketItIsAnsweringOn(t *testing.T) {
	c := socketFor(t)
	raw, err := c.Call("session.hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	var h Hello
	if err := json.Unmarshal(raw, &h); err != nil {
		t.Fatal(err)
	}
	if h.Socket != c.Path() {
		t.Fatalf("hello says %s, the client is on %s", h.Socket, c.Path())
	}
}

// An unknown verb says so, by code. A client's first move after a version
// mismatch is to tell one apart from a genuine fault.
func TestUnknownVerbIsCoded(t *testing.T) {
	c := socketFor(t)
	_, err := c.Call("no.such.verb", nil)
	if err == nil {
		t.Fatal("an unknown verb answered")
	}
	if got := control.CodeOf(err); got != control.UnknownVerb {
		t.Errorf("code %q, want %q (%v)", got, control.UnknownVerb, err)
	}
	// And the message still names it, which is what a person needs.
	if !contains(err.Error(), "no.such.verb") {
		t.Errorf("the refusal does not name the verb: %v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
