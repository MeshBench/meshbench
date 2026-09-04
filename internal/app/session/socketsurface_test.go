package session

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/control"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// What a stranger can reach.
//
// The socket used to pass everything it did not answer itself straight to the
// store, so the workbench's own callbacks were part of the public surface -
// enumerated by session.hello and callable by anybody. They take Go values a
// worker hands them; over the wire the type assertion missed, the zero value
// was applied over a real answer, and success was returned for it.

// wholeSessionSocket is a full session, served on a socket of its own, which is
// how a client meets it.
func wholeSessionSocket(t *testing.T) (*control.Client, *state.Store, *Sim) {
	t.Helper()
	st, sm := Boot(Options{NoPrefs: true, Headless: true})
	ctx, cancel := context.WithCancel(context.Background())
	go st.Run(ctx)
	t.Cleanup(cancel)

	path := filepath.Join(t.TempDir(), "surface.sock")
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
	return c, st, sm
}

// Neither answer the socket gives about itself offers a callback, and the two
// agree with each other: a client that enumerates the verbs and calls what it
// found never meets a refusal it could not have predicted.
func TestTheSocketNeverOffersACallback(t *testing.T) {
	c, st, _ := wholeSessionSocket(t)

	internal := map[string]bool{}
	for _, v := range st.InternalVerbs() {
		internal[v] = true
	}
	if len(internal) == 0 {
		t.Fatal("no verb is registered internal, so this test proves nothing")
	}

	raw, err := c.Call("session.verbs", nil)
	if err != nil {
		t.Fatalf("session.verbs: %v", err)
	}
	var listed struct {
		Verbs []string `json:"verbs"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatal(err)
	}
	for _, v := range listed.Verbs {
		if internal[v] {
			t.Errorf("session.verbs offers %s, which the socket refuses", v)
		}
	}
	if len(listed.Verbs) != len(st.PublicVerbs()) {
		t.Errorf("session.verbs lists %d, the store has %d public",
			len(listed.Verbs), len(st.PublicVerbs()))
	}

	raw, err = c.Call("session.hello", nil)
	if err != nil {
		t.Fatalf("session.hello: %v", err)
	}
	var h Hello
	if err := json.Unmarshal(raw, &h); err != nil {
		t.Fatal(err)
	}
	if h.Verbs != len(listed.Verbs) {
		t.Errorf("hello counts %d verbs, session.verbs lists %d",
			h.Verbs, len(listed.Verbs))
	}
}
