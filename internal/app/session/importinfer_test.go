package session

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/provider"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A packet names the nodes on its path by public key - CoreScope's
// resolved_path is keys, not display names - so the inference comes back keyed
// by key. infer.apply has to match nodes on that key; matching on the display
// name credited almost nobody and a whole import came back a silent mesh
// (regions inferred from thousands of packets, applied to a handful). This pins
// the key match so that regression cannot return.
func TestInferApplyMatchesByPublicKey(t *testing.T) {
	nodes := []scenario.Node{
		{Name: "Belfast Central", PublicKey: "aaaa1111"},
		{Name: "snn-redwood-msi", PublicKey: "bbbb2222"},
		{Name: "no-key-placed"}, // a hand-placed node with no key
	}
	st := state.New(10)
	s := &Sim{nodes: nodes}
	// Inference as the reader produces it: keyed by public key, with named
	// regions. Only the two imported nodes appear; the placed one never sent a
	// packet, so it is absent, as it would be in a real read.
	s.imp = &importState{inferred: map[string]*provider.Inferred{
		"aaaa1111": {Node: "aaaa1111", Regions: []string{"#ioi"}, DefaultScope: "#ioi"},
		"bbbb2222": {Node: "bbbb2222", Regions: []string{"#sco", "#ioi"}},
	}}
	registerImport(st, s)
	go st.Run(t.Context())

	got, err := st.Do(t.Context(), "infer.apply", nil)
	if err != nil {
		t.Fatalf("infer.apply: %v", err)
	}
	if n := got.(map[string]any)["applied"].(int); n != 2 {
		t.Fatalf("applied to %d nodes, want 2 - matching by name instead of key "+
			"would have applied to 0", n)
	}
	if r := s.nodes[0].Regions; len(r) != 1 || r[0] != "#ioi" {
		t.Errorf("Belfast Central regions = %v, want [#ioi]", r)
	}
	if s.nodes[0].DefaultScope != "#ioi" {
		t.Errorf("Belfast Central default scope = %q, want #ioi", s.nodes[0].DefaultScope)
	}
	if r := s.nodes[1].Regions; len(r) != 2 {
		t.Errorf("snn-redwood-msi regions = %v, want two", r)
	}
	if len(s.nodes[2].Regions) != 0 {
		t.Errorf("the placed node with no key and no traffic got regions: %v", s.nodes[2].Regions)
	}
}

// And the map has to colour the same nodes the verb counted.
//
// The snapshot was written by a second walk of its own, and because a row
// carries no public key that walk matched on the display name - the very match
// the key lookup above exists to replace. So the verb answered 44 and four
// nodes changed colour, on three separate imports. The count is what both
// walks agreed on, so this asserts on which nodes carry which regions.
func TestInferApplyColoursTheNodesItCounted(t *testing.T) {
	nodes := []scenario.Node{
		{Name: "Belfast Central", PublicKey: "aaaa1111"},
		{Name: "snn-redwood-msi", PublicKey: "bbbb2222"},
		{Name: "no-key-placed"},
	}
	st := state.New(10)
	s := &Sim{nodes: nodes}
	s.imp = &importState{inferred: map[string]*provider.Inferred{
		"aaaa1111": {Node: "aaaa1111", Regions: []string{"#ioi"}, DefaultScope: "#ioi"},
		"bbbb2222": {Node: "bbbb2222", Regions: []string{"#sco", "#ioi"}},
	}}
	registerImport(st, s)
	st.Handle("test.nodes", func(w *state.World, _ any) (any, error) {
		for _, n := range nodes {
			w.Nodes = append(w.Nodes, state.Node{Name: n.Name})
		}
		return nil, nil
	})
	var world *state.World
	st.Handle("test.world", func(w *state.World, p any) (any, error) {
		*(p.(**state.World)) = w
		return nil, nil
	})
	go st.Run(t.Context())
	if _, err := st.Do(t.Context(), "test.nodes", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Do(t.Context(), "infer.apply", nil); err != nil {
		t.Fatalf("infer.apply: %v", err)
	}
	if _, err := st.Do(t.Context(), "test.world", &world); err != nil {
		t.Fatal(err)
	}

	want := map[string][]string{
		"Belfast Central": {"#ioi"},
		"snn-redwood-msi": {"#sco", "#ioi"},
		"no-key-placed":   nil,
	}
	for _, row := range world.Nodes {
		if got := row.Regions; !sameStrings(got, want[row.Name]) {
			t.Errorf("the map draws %s holding %v, want %v - the row was matched "+
				"on its display name where the scenario was matched on its key",
				row.Name, got, want[row.Name])
		}
	}
	for _, row := range world.Nodes {
		if row.Name == "Belfast Central" && row.DefaultScope != "#ioi" {
			t.Errorf("Belfast Central is drawn originating under %q, want #ioi",
				row.DefaultScope)
		}
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
