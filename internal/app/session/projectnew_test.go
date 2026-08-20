package session

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A blank network to build one by hand. There was no way to get one: every
// session began with a fixture, and the only way to an empty map was to open
// something and delete it.
func TestANewNetworkIsEmptyAndStillRuns(t *testing.T) {
	st := state.New(10)
	Register(st, &Sim{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	if _, err := st.Do(ctx, "project.open", "../../../fixtures/fixture-fife-strict.json"); err != nil {
		t.Skip("no fixture:", err)
	}
	if len(st.Snapshot().Nodes) == 0 {
		t.Fatal("the fixture loaded no nodes, so starting blank proves nothing")
	}
	if _, err := st.Do(ctx, "project.new", nil); err != nil {
		t.Fatal(err)
	}
	s := st.Snapshot()
	if len(s.Nodes) != 0 || len(s.Links) != 0 || len(s.Areas) != 0 {
		t.Fatalf("a blank network still holds %d nodes, %d links, %d areas",
			len(s.Nodes), len(s.Links), len(s.Areas))
	}
	// And it is a running session, not a dead one: an empty engine still
	// steps, so the clock and everything paced by it keep working.
	if _, err := st.Do(ctx, "sim.step", nil); err != nil {
		t.Fatalf("stepping a blank network: %v", err)
	}
	// A node can then be placed into it, which is the whole point.
	if _, err := st.Do(ctx, "nodes.place", map[string]any{
		"name": "first", "kind": "simple-repeater", "lat": 56.3, "lon": -3.3,
	}); err != nil {
		t.Fatalf("placing the first node: %v", err)
	}
	if got := len(st.Snapshot().Nodes); got != 1 {
		t.Fatalf("after placing one node the network holds %d", got)
	}
}

// centreOf points the camera at a named place, because a network with no
// nodes has nothing to frame and framing nothing lands in the Atlantic.
func TestCentreOfAnOutline(t *testing.T) {
	lat, lon, ok := centreOf([]scenario.Boundary{{Rings: []scenario.Ring{{
		{Lat: 56.0, Lon: -4.0}, {Lat: 57.0, Lon: -4.0},
		{Lat: 57.0, Lon: -2.0}, {Lat: 56.0, Lon: -2.0},
	}}}})
	if !ok || lat != 56.5 || lon != -3.0 {
		t.Fatalf("centre is %v,%v (ok=%v), want the middle of the outline", lat, lon, ok)
	}
	if _, _, ok := centreOf(nil); ok {
		t.Error("an empty outline reported a centre")
	}
}

// A country with far-flung territories centres on the part somebody means.
//
// France reported 0,0 - open ocean off west Africa - because its boundary
// takes in Guadeloupe, French Guiana and Réunion and the middle of that
// extent is nowhere near France.
func TestCentreOfIgnoresTheFarFlungParts(t *testing.T) {
	mainland := make(scenario.Ring, 0, 64)
	for i := 0; i < 64; i++ {
		mainland = append(mainland, scenario.LatLon{
			Lat: 46 + float64(i%8)*0.5, Lon: 2 + float64(i%8)*0.5})
	}
	overseas := scenario.Ring{ // Réunion, four points
		{Lat: -21.0, Lon: 55.2}, {Lat: -21.3, Lon: 55.2},
		{Lat: -21.3, Lon: 55.8}, {Lat: -21.0, Lon: 55.8},
	}
	lat, lon, ok := centreOf([]scenario.Boundary{{Rings: []scenario.Ring{overseas, mainland}}})
	if !ok {
		t.Fatal("no centre for a boundary with two rings")
	}
	if lat < 40 || lon < -5 || lon > 15 {
		t.Fatalf("centre is %v,%v - that is not in France", lat, lon)
	}
}
