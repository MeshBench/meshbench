package boundary_test

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	_ "github.com/MeshBench/meshbench/internal/app/session/boundary"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// A square polygon named in its own properties, the offline path boundary.load
// takes: no Nominatim, no network, just GeoJSON in and a study area out.
const testGeoJSON = `{"type":"FeatureCollection","features":[` +
	`{"type":"Feature","properties":{"name":"Testland"},` +
	`"geometry":{"type":"Polygon","coordinates":[[[-4,56],[-3,56],[-3,57],[-4,57],[-4,56]]]}}]}`

// boundary.load parses a GeoJSON study area into the world, and boundary.list
// reports what the study area is made of - pinned here because the verbs moved
// out of session into this package and must still register and behave.
func TestBoundaryLoadAndListRoundTrip(t *testing.T) {
	store := state.New(10)
	sim := &session.Sim{}
	session.Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)

	got, err := store.Do(ctx, "boundary.load", map[string]any{"geojson": testGeoJSON})
	if err != nil {
		t.Fatalf("boundary.load: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("boundary.load returned %T, want a map", got)
	}
	if n, _ := m["areas"].(int); n != 1 {
		t.Fatalf("after load, areas = %v, want 1", m["areas"])
	}
	if n, _ := m["polygons"].(int); n != 1 {
		t.Fatalf("polygons = %v, want 1", m["polygons"])
	}
	loaded, _ := m["loaded"].([]string)
	if len(loaded) != 1 || loaded[0] != "Testland" {
		t.Fatalf("loaded = %v, want [Testland]", m["loaded"])
	}

	got, err = store.Do(ctx, "boundary.list", nil)
	if err != nil {
		t.Fatalf("boundary.list: %v", err)
	}
	m = got.(map[string]any)
	names, _ := m["names"].([]string)
	if len(names) != 1 || names[0] != "Testland" {
		t.Fatalf("boundary.list names = %v, want [Testland]", m["names"])
	}
}
