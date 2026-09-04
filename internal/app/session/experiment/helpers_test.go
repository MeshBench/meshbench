// What the matrix's own tests set up: a registered verb set over a small
// network, and the two readers that assert on a refusal.
//
// Deliberately a copy of the shape session's own refusal tests use rather than
// a shared export. A test helper that reaches into a Sim is the one thing that
// should not be exported for a domain package's benefit: it exists to write
// unexported fields, and exporting it would put a setter for them on the
// public surface for ever.
package experiment

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// aNetwork is the whole verb set over two named nodes.
//
// The names are two real ScotMesh sites, as in session's own refusal tests,
// because a sender is named and a name that is nearly right is what these
// verbs used to accept.
func aNetwork(t *testing.T) (*state.Store, *session.Sim) {
	t.Helper()
	// Every verb in the tree is reachable from here and some of them write, so
	// the caches and configuration are pointed at a temporary home rather than
	// the developer's own.
	home := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", home)
	t.Setenv("LOCALAPPDATA", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("APPDATA", home)

	st := state.New(10)
	s := &session.Sim{}
	session.Register(st, s)
	go st.Run(t.Context())

	nodes := []scenario.Node{
		{Name: "West Lomond", Kind: scenario.SimpleRepeater},
		{Name: "Dunfermline", Kind: scenario.SimpleRepeater},
	}
	nodes[0].Position.Lat, nodes[0].Position.Lon = 56.25, -3.29
	nodes[1].Position.Lat, nodes[1].Position.Lon = 56.07, -3.46
	s.BuildSeeded(nodes, 869.618, 1)
	return st, s
}

func refuses(t *testing.T, st *state.Store, verb string, params any) string {
	t.Helper()
	got, err := st.Do(t.Context(), verb, params)
	if err == nil {
		t.Fatalf("%s accepted %v and answered %v", verb, params, got)
	}
	return err.Error()
}

func mentions(t *testing.T, msg string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(msg, w) {
			t.Errorf("%q does not mention %q", msg, w)
		}
	}
}
