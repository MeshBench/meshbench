package session

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/boundary"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// The study area the panels show is the one the import filters by.
//
// Opening a network replaced the world's list of areas and left the geometry
// the import reads alone. The workbench opens Scotland and Ireland at startup,
// so the Import panel said "study area: Scotland, Ireland - nodes outside it are
// not imported" and a fetch then brought in every node the deployment had; and
// a blank network started after Fife had been accepted said there was no study
// area while the fetch still kept only Fife.

func runningSession(t *testing.T) (*state.Store, *Sim, context.Context) {
	t.Helper()
	st := state.New(10)
	s := &Sim{}
	Register(st, s)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go st.Run(ctx)
	return st, s, ctx
}

func TestAnOpenedNetworksAreasBoundTheImport(t *testing.T) {
	st, s, ctx := runningSession(t)
	if _, err := st.Do(ctx, "project.open",
		"../../../fixtures/fixture-scotland-ireland-strict.json"); err != nil {
		t.Skip("no fixture:", err)
	}
	shown := st.Snapshot().Areas
	if len(shown) == 0 {
		t.Fatal("the fixture opened with no study area, so this proves nothing")
	}
	o := importOptions(s, 30)
	if o.Region == nil {
		t.Fatalf("the network shows %d study areas and the import is not narrowed at all",
			len(shown))
	}
	edinburgh := scenario.LatLon{Lat: 55.95, Lon: -3.19}
	sydney := scenario.LatLon{Lat: -33.87, Lon: 151.21}
	if !o.Region.Contains(edinburgh) {
		t.Error("Edinburgh is not inside a study area of Scotland and Ireland")
	}
	if o.Region.Participates(sydney) {
		t.Error("Sydney would be imported into a study of Scotland and Ireland")
	}

	// Each opened area can still be taken back out, geometry with it.
	for _, a := range shown {
		if _, err := st.Do(ctx, "boundary.remove", map[string]any{"name": a.Name}); err != nil {
			t.Fatalf("removing %s: %v", a.Name, err)
		}
	}
	if len(s.areas) != 0 {
		t.Fatalf("every area was removed and %d boundaries still filter the import",
			len(s.areas))
	}
}

func TestABlankNetworkForgetsTheLastStudyArea(t *testing.T) {
	st, s, ctx := runningSession(t)
	s.foundAreas = []boundary.Found{{Name: "Fife",
		Boundaries: []scenario.Boundary{{Name: "Fife", Rings: []scenario.Ring{{
			{Lat: 56.0, Lon: -3.6}, {Lat: 56.5, Lon: -3.6},
			{Lat: 56.5, Lon: -2.7}, {Lat: 56.0, Lon: -2.7}, {Lat: 56.0, Lon: -3.6},
		}}}}}}
	if _, err := st.Do(ctx, "boundary.accept", map[string]any{"name": "Fife"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Do(ctx, "project.new", nil); err != nil {
		t.Fatal(err)
	}
	if n := len(st.Snapshot().Areas); n != 0 {
		t.Fatalf("a blank network shows %d study areas", n)
	}
	if o := importOptions(s, 30); o.Region != nil {
		t.Fatal("a blank network shows no study area and still narrows the import to Fife")
	}
}
