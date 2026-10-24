package session

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

func presetSim(t *testing.T, nodes []scenario.Node) (*state.Store, *Sim, context.Context, func()) {
	t.Helper()
	st := state.New(10)
	s := &Sim{gpuAsked: true}
	Register(st, s)
	ctx, cancel := context.WithCancel(context.Background())
	go st.Run(ctx)
	s.build(nodes, 869.618)
	return st, s, ctx, cancel
}

// A preset puts a node on the preset's coding rate. The table holds MeshCore's
// denominator, and copying it straight into the node's radio made EU/UK
// (Narrow) 4/12.
func TestAPresetKeepsItsCodingRate(t *testing.T) {
	st, s, ctx, cancel := presetSim(t, []scenario.Node{repeaterNode("Alpha")})
	defer cancel()
	if _, err := st.Do(ctx, "radio.preset", map[string]any{"preset": "EU/UK (Narrow)"}); err != nil {
		t.Fatal(err)
	}
	got := s.nodes[0].Radio
	want, _ := scenario.PresetByLabel("EU/UK (Narrow)")
	if !want.Matches(got) {
		t.Fatalf("after the preset the node's radio is %+v, which is not EU/UK (Narrow)", got)
	}
	if got.CodingRate != 4 {
		t.Fatalf("coding rate field %d, want 4 (4/8)", got.CodingRate)
	}
}

// A placed node joins on the modem the mesh already shares, rather than on
// the engine's fallback that nothing else is using.
func TestAPlacedNodeJoinsTheMeshItIsPlacedIn(t *testing.T) {
	nl, _ := scenario.PresetByLabel("Netherlands")
	a, b := repeaterNode("Alpha"), repeaterNode("Bravo")
	a.Radio, b.Radio = nl.Config(), nl.Config()
	st, s, ctx, cancel := presetSim(t, []scenario.Node{a, b})
	defer cancel()
	if _, err := st.Do(ctx, "nodes.place", map[string]any{
		"name": "Charlie", "kind": "companion", "lat": 56.31, "lon": -3.21,
	}); err != nil {
		t.Fatal(err)
	}
	for _, n := range s.nodes {
		if n.Name == "Charlie" && n.Radio != nl.Config() {
			t.Fatalf("Charlie was placed on %+v, the mesh is on %+v", n.Radio, nl.Config())
		}
	}
}

// With nothing to join, or neighbours with no majority, a placed node gets the
// default the presets table documents.
func TestAPlacedNodeOnNoAgreedModemGetsTheDefault(t *testing.T) {
	if got := placedRadio(nil, 56, -3); got != scenario.DefaultRadio() {
		t.Fatalf("an empty mesh placed on %+v", got)
	}
	nl, _ := scenario.PresetByLabel("Netherlands")
	uk, _ := scenario.PresetByLabel("EU/UK (Narrow)")
	tied := []scenario.Node{
		{Radio: nl.Config(), Position: scenario.LatLon{Lat: 56, Lon: -3}},
		{Radio: uk.Config(), Position: scenario.LatLon{Lat: 56, Lon: -3}},
	}
	if got := placedRadio(tied, 56, -3); got != scenario.DefaultRadio() {
		t.Fatalf("a tied mesh placed on %+v", got)
	}
}

// In a mixed mesh a placed node joins the side it was placed on.
func TestAPlacedNodeJoinsItsNeighboursInAMixedMesh(t *testing.T) {
	nl, _ := scenario.PresetByLabel("Netherlands")
	uk, _ := scenario.PresetByLabel("EU/UK (Narrow)")
	var mesh []scenario.Node
	// Six Dutch nodes and ten Scottish ones: the mesh as a whole is mostly
	// Scottish, and the ten nearest the Dutch spot are mostly Dutch.
	for i := 0; i < 6; i++ {
		mesh = append(mesh, scenario.Node{Radio: nl.Config(),
			Position: scenario.LatLon{Lat: 52.0 + float64(i)*0.01, Lon: 5.0}})
	}
	for i := 0; i < 10; i++ {
		mesh = append(mesh, scenario.Node{Radio: uk.Config(),
			Position: scenario.LatLon{Lat: 56.0 + float64(i)*0.01, Lon: -3.0}})
	}
	if got := placedRadio(mesh, 52.01, 5.0); got != nl.Config() {
		t.Fatalf("a node placed among the Dutch nodes joined %+v", got)
	}
	if got := placedRadio(mesh, 56.01, -3.0); got != uk.Config() {
		t.Fatalf("a node placed among the Scottish nodes joined %+v", got)
	}
}
