package session

import (
	"context"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/world/boundary"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A study area accepted before the import is applied by the import.
//
// Workbench1 passed the region into the fetch; this stopped, so a national
// feed came in whole and the only way to narrow it was to commit all of it
// and prune - which measures every pair of four hundred nodes against the
// terrain and the buildings before throwing most of the answer away.
func TestTheStudyAreaBoundsTheImport(t *testing.T) {
	s := &Sim{}
	// A square around Fife, roughly.
	s.areas = []scenario.Boundary{{Rings: []scenario.Ring{{
		{Lat: 56.0, Lon: -3.6}, {Lat: 56.5, Lon: -3.6},
		{Lat: 56.5, Lon: -2.7}, {Lat: 56.0, Lon: -2.7}, {Lat: 56.0, Lon: -3.6},
	}}}}

	o := importOptions(s, 30)
	if o.Region == nil {
		t.Fatal("an accepted study area did not reach the import options")
	}
	if got := o.Region.MarginKm; got != 30 {
		t.Fatalf("margin %v reached the import, want the world's 30", got)
	}
	in := scenario.LatLon{Lat: 56.2, Lon: -3.2}   // inside
	far := scenario.LatLon{Lat: 51.5, Lon: -0.12} // London
	if !o.Region.Contains(in) {
		t.Error("a node inside the area is not contained by the region")
	}
	if o.Region.Participates(far) {
		t.Error("a node 500 km away participates; the area bounds nothing")
	}

	// And with nothing accepted the import is unbounded, as it was.
	if got := importOptions(&Sim{}, 30); got.Region != nil {
		t.Error("with no study area accepted the import is narrowed anyway")
	}
}

// The margin falls back to the scenario default rather than to zero: a zero
// margin drops every repeater just outside the border, which makes the mesh
// behave better than reality.
func TestTheImportMarginFallsBackToTheDefault(t *testing.T) {
	s := &Sim{areas: []scenario.Boundary{{Rings: []scenario.Ring{{
		{Lat: 56.0, Lon: -3.6}, {Lat: 56.5, Lon: -3.6},
		{Lat: 56.5, Lon: -2.7}, {Lat: 56.0, Lon: -2.7}, {Lat: 56.0, Lon: -3.6},
	}}}}}
	if got := importOptions(s, 0).Region.MarginKm; got != scenario.DefaultMarginKm {
		t.Fatalf("margin %v with none set, want the %v default", got, scenario.DefaultMarginKm)
	}
}

// import.describe reports what the commit would keep, not what the feed
// published: a description of a different import is worse than none.
func TestDescribeRefusesWithoutASource(t *testing.T) {
	st := state.New(10)
	Register(st, &Sim{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)
	if _, err := ImportFrom(ctx, "", nil); err == nil {
		t.Fatal("an import with no URL was accepted")
	}
}

// A study area is built from several places, and one can be taken back out.
// Scotland and Ireland is one study; removing Ireland leaves Scotland.
func TestTheStudyAreaTakesSeveralPlacesAndGivesThemBack(t *testing.T) {
	st := state.New(10)
	s := &Sim{}
	Register(st, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	// Two areas accepted, as two searches would leave them.
	area := func(name string, lat float64) {
		s.foundAreas = append(s.foundAreas, boundary.Found{Name: name,
			Boundaries: []scenario.Boundary{{Name: name, Rings: []scenario.Ring{{
				{Lat: lat, Lon: -4.0}, {Lat: lat + 1, Lon: -4.0},
				{Lat: lat + 1, Lon: -2.0}, {Lat: lat, Lon: -2.0}, {Lat: lat, Lon: -4.0},
			}}}}})
		if _, err := st.Do(ctx, "boundary.accept", map[string]any{"name": name}); err != nil {
			t.Fatalf("accepting %s: %v", name, err)
		}
	}
	area("Scotland", 56)
	area("Ireland", 53)
	if got := len(st.Snapshot().Areas); got != 2 {
		t.Fatalf("the study holds %d areas, want both", got)
	}
	// Accepting one twice does not stack it.
	if _, err := st.Do(ctx, "boundary.accept", map[string]any{"name": "Scotland"}); err != nil {
		t.Fatal(err)
	}
	if got := len(st.Snapshot().Areas); got != 2 {
		t.Fatalf("accepting Scotland twice left %d areas", got)
	}
	// And one comes back out, geometry with it.
	if _, err := st.Do(ctx, "boundary.remove", map[string]any{"name": "Ireland"}); err != nil {
		t.Fatal(err)
	}
	snap := st.Snapshot()
	if len(snap.Areas) != 1 || snap.Areas[0].Name != "Scotland" {
		t.Fatalf("after removing Ireland the study is %v", snap.Areas)
	}
	if len(s.areas) != 1 || !strings.EqualFold(s.areas[0].Name, "Scotland") {
		t.Fatalf("the geometry still holds %d boundaries; it must follow the list", len(s.areas))
	}
	// Removing one that is not there says what there is.
	if _, err := st.Do(ctx, "boundary.remove", map[string]any{"name": "Wales"}); err == nil {
		t.Error("removing an area that is not in the study was accepted")
	}
}
