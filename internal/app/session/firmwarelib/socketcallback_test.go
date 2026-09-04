// The callback the published-build fetch answers on, and what the control
// socket is allowed to do with it.
//
// These live here rather than with session's other socket-surface tests
// because the catalogue they assert on does: the rule is the socket's, but the
// only way to see a refused callback change nothing is to look at the state it
// would have changed.
package firmwarelib

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/control"
	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// wholeSessionSocket is a booted session with the control socket in front of
// it, which is the only way to ask what a socket client can reach.
func wholeSessionSocket(t *testing.T) (*control.Client, *state.Store, *session.Sim) {
	t.Helper()
	st, sm := session.Boot(session.Options{NoPrefs: true, Headless: true})
	ctx, cancel := context.WithCancel(context.Background())
	go st.Run(ctx)
	t.Cleanup(cancel)

	path := filepath.Join(t.TempDir(), "surface.sock")
	srv, err := session.ServeControlAt(ctx, st, path)
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

func contains(s, sub string) bool { return strings.Contains(s, sub) }

// The refusal that matters: the catalogue a fetch landed is still there
// afterwards. This is the shape of the original fault - the call succeeded and
// left the library empty.
func TestACallbackFromTheSocketChangesNothing(t *testing.T) {
	c, _, sm := wholeSessionSocket(t)
	catalogueOf(sm).published = []publishedBuild{{role: "companion", version: "v1.17.0"}}

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
	if len(catalogueOf(sm).published) != 1 {
		t.Errorf("the catalogue is %d builds long; the refusal was not free",
			len(catalogueOf(sm).published))
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
	st, sm := session.Boot(session.Options{NoPrefs: true, Headless: true})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	catalogueOf(sm).published = []publishedBuild{{role: "companion", version: "v1.17.0"}}
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
	if len(catalogueOf(sm).published) != 1 {
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
	if len(catalogueOf(sm).published) != 0 {
		t.Error("the fetch's own answer did not land")
	}
}
