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

// The refusal that matters: the catalogue a fetch landed is still there
// afterwards. This is the shape of the original fault - the call succeeded and
// left the library empty.
func TestACallbackFromTheSocketChangesNothing(t *testing.T) {
	c, _, sm := wholeSessionSocket(t)
	sm.publishedNet = []publishedBuild{{role: "companion", version: "v1.17.0"}}

	_, err := c.Call("firmware.published", nil)
	if err == nil {
		t.Fatal("firmware.published answered a socket client")
	}
	if got := control.CodeOf(err); got != control.BadParams {
		t.Errorf("code %q, want %q (%v)", got, control.BadParams, err)
	}
	if !contains(err.Error(), "firmware.published") {
		t.Errorf("the refusal does not name the verb: %v", err)
	}
	if len(sm.publishedNet) != 1 {
		t.Errorf("the catalogue is %d builds long; the refusal was not free",
			len(sm.publishedNet))
	}

	// And one carrying an object it might plausibly decode, so the refusal is
	// about the verb rather than about the parameters that arrived.
	if _, err := c.Call("coverage.set", map[string]any{"node": "GB7XYZ"}); err == nil {
		t.Error("coverage.set answered a socket client")
	}
}

// An internal caller that passes the wrong thing is told, rather than having a
// zero value applied on its behalf.
func TestACallbackHandedTheWrongValueRefuses(t *testing.T) {
	st, sm := Boot(Options{NoPrefs: true, Headless: true})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	sm.publishedNet = []publishedBuild{{role: "companion", version: "v1.17.0"}}
	for _, c := range []struct {
		verb string
		p    any
	}{
		{"firmware.published", map[string]any{"builds": 3}},
		{"links.set", "every link"},
		{"job.progress", map[string]any{"id": "warm"}},
		{"coverage.set", map[string]any{}},
		{"import.set", nil},
	} {
		if _, err := st.Do(ctx, c.verb, c.p); err == nil {
			t.Errorf("%s accepted %v", c.verb, c.p)
		}
	}
	if len(sm.publishedNet) != 1 {
		t.Error("a refused callback overwrote the catalogue anyway")
	}
	if snap := st.Snapshot(); snap.Coverage != nil || snap.Import != nil {
		t.Error("a refused callback published something")
	}

	// The value its own worker passes is still accepted, or the check has
	// broken the callback it was meant to protect.
	if _, err := st.Do(ctx, "firmware.published", []publishedBuild{}); err != nil {
		t.Errorf("the fetch's own callback was refused: %v", err)
	}
	if len(sm.publishedNet) != 0 {
		t.Error("the fetch's own answer did not land")
	}
}
