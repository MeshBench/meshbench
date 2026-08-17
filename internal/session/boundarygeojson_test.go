package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// Fife, roughly - one ring, a plain Polygon feature, the shape a tool like
// geojson.io actually exports.
const fifeGeoJSON = `{
  "type": "Feature",
  "properties": {"name": "Fife group coverage"},
  "geometry": {
    "type": "Polygon",
    "coordinates": [[
      [-3.4, 56.0], [-2.6, 56.0], [-2.6, 56.4], [-3.4, 56.4], [-3.4, 56.0]
    ]]
  }
}`

func newBoundaryTestSim(t *testing.T, nodes []scenario.Node) (*state.Store, *Sim) {
	t.Helper()
	st := state.New(10)
	s := &Sim{gpuAsked: true}
	Register(st, s)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go st.Run(ctx)
	s.build(nodes, 869.618)
	return st, s
}

func TestBoundaryImportAddsAnArea(t *testing.T) {
	inside := repeaterNode("Inside Fife")
	inside.Position = scenario.LatLon{Lat: 56.2, Lon: -3.0}
	outside := repeaterNode("Outside Fife")
	outside.Position = scenario.LatLon{Lat: 60.0, Lon: -1.0}
	st, s := newBoundaryTestSim(t, []scenario.Node{inside, outside})
	_ = s

	path := filepath.Join(t.TempDir(), "fife.geojson")
	if err := os.WriteFile(path, []byte(fifeGeoJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	res, err := st.Do(ctx, "boundary.import", map[string]any{"path": path})
	if err != nil {
		t.Fatalf("boundary.import: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("got %T", res)
	}
	if m["name"] != "Fife group coverage" {
		t.Errorf("name carried %v, wanted the feature's own property", m["name"])
	}
	if got, _ := m["nodes_inside"].(int); got != 1 {
		t.Errorf("nodes_inside: got %v, wanted 1 - only the node actually in the polygon", m["nodes_inside"])
	}

	snap := st.Snapshot()
	if len(snap.Areas) != 1 {
		t.Fatalf("got %d areas, wanted 1", len(snap.Areas))
	}
	if snap.Areas[0].Source != "geojson" {
		t.Errorf("source: got %q, wanted %q - so it can be told apart from a search result", snap.Areas[0].Source, "geojson")
	}
}

func TestBoundaryImportRejectsAFileThatIsNotGeoJSON(t *testing.T) {
	st, _ := newBoundaryTestSim(t, nil)
	path := filepath.Join(t.TempDir(), "not-geojson.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Do(context.Background(), "boundary.import",
		map[string]any{"path": path}); err == nil {
		t.Fatal("wanted a refusal, got none")
	}
}

func TestBoundaryImportRejectsAMissingFile(t *testing.T) {
	st, _ := newBoundaryTestSim(t, nil)
	if _, err := st.Do(context.Background(), "boundary.import",
		map[string]any{"path": "/nonexistent/nowhere.geojson"}); err == nil {
		t.Fatal("wanted a refusal, got none")
	}
}

// Zero nodes inside a non-empty network is nearly always a lon/lat swap - the
// note field is where that gets said, since a silent zero looks identical to
// a boundary that is simply somewhere else.
func TestBoundaryImportNotesWhenNothingFallsInside(t *testing.T) {
	elsewhere := repeaterNode("Elsewhere")
	elsewhere.Position = scenario.LatLon{Lat: 10, Lon: 10}
	st, _ := newBoundaryTestSim(t, []scenario.Node{elsewhere})

	path := filepath.Join(t.TempDir(), "fife.geojson")
	if err := os.WriteFile(path, []byte(fifeGeoJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := st.Do(context.Background(), "boundary.import", map[string]any{"path": path})
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]any)
	if m["note"] == "" {
		t.Error("zero nodes inside a non-empty network should say why that might be")
	}
}
