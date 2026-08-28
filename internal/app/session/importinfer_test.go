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
