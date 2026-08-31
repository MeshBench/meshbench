package environ_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/environ"
)

const fixtureGeoJSONL = `{"type":"Feature","geometry":{"type":"Polygon","coordinates":[[[-2.790,56.340],[-2.789,56.340],[-2.789,56.341],[-2.790,56.341],[-2.790,56.340]]]},"properties":{"height":14.5,"building":"industrial"}}
{"type":"Feature","geometry":{"type":"Polygon","coordinates":[[[-2.795,56.342],[-2.794,56.342],[-2.794,56.343],[-2.795,56.343],[-2.795,56.342]]]},"properties":{"building":"house","building:levels":"2","building:material":"stone"}}
`

// The whole ingestion path: GeoJSONL through IngestGeoJSON into tiles the
// store loads back, with heights and materials carrying their provenance.
func TestIngestGeoJSONRoundTrips(t *testing.T) {
	dir := t.TempDir()
	stats, err := environ.IngestGeoJSON(strings.NewReader(fixtureGeoJSONL), dir, "uk")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Buildings != 2 || stats.Skipped != 0 {
		t.Fatalf("stats %+v", stats)
	}

	s := environ.OpenTiles(dir)
	got := s.Buildings(56.339, -2.796, 56.344, -2.788)
	if len(got) != 2 {
		t.Fatalf("expected 2 buildings back, got %d", len(got))
	}
	byType := map[string]environ.Building{}
	for _, b := range got {
		byType[b.Type] = b
	}
	ind := byType["industrial"]
	if ind.HeightM != 14.5 || ind.HeightSource != "dataset" || ind.Material != environ.MatMetal {
		t.Fatalf("industrial mangled: %+v", ind)
	}
	house := byType["house"]
	if house.HeightM != 6 || house.HeightSource != "levels" ||
		house.Material != environ.MatStone || house.MaterialSource != "osm" {
		t.Fatalf("house mangled: %+v", house)
	}
	// GeoJSON is lon,lat; a swapped import would put Fife in the Indian
	// Ocean, which is exactly the mistake the plan says to catch at the door.
	if fp := ind.Footprint[0]; fp[0] < 50 || fp[0] > 60 {
		t.Fatalf("latitude out of Scotland: %v - lon/lat swapped?", fp)
	}
}

// A short but complete FeatureCollection fits inside the 512-byte window the
// format sniff used to peek: read whole, it was valid JSON on its own, and
// got scanned as a single NDJSON line that could never unmarshal as one
// feature - skipped, not ingested.
const fixtureShortFeatureCollection = `{"type":"FeatureCollection","features":[` +
	`{"type":"Feature","geometry":{"type":"Polygon","coordinates":` +
	`[[[-2.790,56.340],[-2.789,56.340],[-2.789,56.341],[-2.790,56.341],[-2.790,56.340]]]},` +
	`"properties":{"height":14.5,"building":"industrial"}}]}`

func TestIngestShortFeatureCollection(t *testing.T) {
	if len(fixtureShortFeatureCollection) >= 512 {
		t.Fatalf("fixture is %d bytes, must stay under the old peek window to be the regression case", len(fixtureShortFeatureCollection))
	}
	dir := t.TempDir()
	stats, err := environ.IngestGeoJSON(strings.NewReader(fixtureShortFeatureCollection), dir, "uk")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Buildings != 1 {
		t.Fatalf("a short FeatureCollection must still ingest its buildings, got %+v", stats)
	}
	s := environ.OpenTiles(dir)
	got := s.Buildings(56.339, -2.796, 56.344, -2.788)
	if len(got) != 1 {
		t.Fatalf("the short FeatureCollection's building did not round-trip, got %d", len(got))
	}
}

// A document that parses cleanly but names no buildings - an empty
// FeatureCollection, or a stream where every line fails to become one - must
// say so rather than reporting success with nothing ingested.
func TestIngestEmptyReportsError(t *testing.T) {
	dir := t.TempDir()
	stats, err := environ.IngestGeoJSON(strings.NewReader(`{"type":"FeatureCollection","features":[]}`), dir, "uk")
	if !errors.Is(err, environ.ErrNoBuildings) {
		t.Fatalf("an empty FeatureCollection must report ErrNoBuildings, got stats=%+v err=%v", stats, err)
	}

	stats, err = environ.IngestGeoJSON(strings.NewReader("not a feature at all\nnor is this\n"), dir, "uk")
	if !errors.Is(err, environ.ErrNoBuildings) {
		t.Fatalf("a stream with no valid features must report ErrNoBuildings, got stats=%+v err=%v", stats, err)
	}
}
