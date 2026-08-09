package scenario_test

import (
	"strings"
	"testing"

	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// GeoJSON is [lon, lat]; everything else in this codebase is lat, lon. Getting
// it the wrong way round puts a Scottish boundary in the Indian Ocean, and the
// polygon is still perfectly valid — it just contains nothing you wanted.
func TestCoordinateOrderIsLonLat(t *testing.T) {
	// A square around Perthshire, written the GeoJSON way.
	doc := `{"type":"Feature","properties":{"name":"test-square"},"geometry":{
		"type":"Polygon","coordinates":[[[-4.2,56.5],[-3.5,56.5],[-3.5,57.0],[-4.2,57.0],[-4.2,56.5]]]}}`

	bs, err := scenario.ParseGeoJSON([]byte(doc), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 1 || bs[0].Name != "test-square" {
		t.Fatalf("parsed %d boundaries: %+v", len(bs), bs)
	}

	r := scenario.Region{Boundaries: bs}
	if !r.Contains(scenario.LatLon{Lat: 56.7, Lon: -3.9}) {
		t.Error("a point inside the square was reported outside — lat and lon are swapped")
	}
	if r.Contains(scenario.LatLon{Lat: -3.9, Lon: 56.7}) {
		t.Error("the swapped point was reported inside, which means the polygon is in the Indian Ocean")
	}
}

// A hole is an exclusion. Treating interior rings as extra outers includes
// exactly the area that was meant to be left out.
func TestHolesAreExcluded(t *testing.T) {
	doc := `{"type":"Polygon","coordinates":[
		[[-4.0,56.0],[-3.0,56.0],[-3.0,57.0],[-4.0,57.0],[-4.0,56.0]],
		[[-3.7,56.3],[-3.3,56.3],[-3.3,56.7],[-3.7,56.7],[-3.7,56.3]]
	]}`
	bs, err := scenario.ParseGeoJSON([]byte(doc), "")
	if err != nil {
		t.Fatal(err)
	}
	r := scenario.Region{Boundaries: bs}

	if !r.Contains(scenario.LatLon{Lat: 56.1, Lon: -3.9}) {
		t.Error("a point in the polygon but outside the hole was excluded")
	}
	if r.Contains(scenario.LatLon{Lat: 56.5, Lon: -3.5}) {
		t.Error("a point inside the hole was included")
	}
}

// Scotland is one boundary made of a mainland and several hundred islands. A
// node on Islay is in Scotland, not in "Scotland part 118".
func TestMultiPolygonPartsShareTheName(t *testing.T) {
	doc := `{"type":"Feature","properties":{"name":"Scotland"},"geometry":{
		"type":"MultiPolygon","coordinates":[
			[[[-4.0,56.0],[-3.0,56.0],[-3.0,57.0],[-4.0,57.0],[-4.0,56.0]]],
			[[[-6.5,55.6],[-6.0,55.6],[-6.0,55.9],[-6.5,55.9],[-6.5,55.6]]]
		]}}`
	bs, err := scenario.ParseGeoJSON([]byte(doc), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 2 {
		t.Fatalf("got %d parts, want 2", len(bs))
	}
	for i, b := range bs {
		if b.Name != "Scotland" {
			t.Errorf("part %d is named %q", i, b.Name)
		}
	}
	r := scenario.Region{Boundaries: bs}
	if !r.Contains(scenario.LatLon{Lat: 55.7, Lon: -6.2}) {
		t.Error("the island part is not being used")
	}
}

// National datasets routinely mix a boundary polygon with a label point.
// Refusing the whole file over the label helps nobody.
func TestLabelPointsAreSkippedNotFatal(t *testing.T) {
	doc := `{"type":"FeatureCollection","features":[
		{"type":"Feature","properties":{"name":"label"},"geometry":{"type":"Point","coordinates":[-3.9,56.7]}},
		{"type":"Feature","properties":{"ctry19nm":"Scotland"},"geometry":{"type":"Polygon",
			"coordinates":[[[-4.0,56.0],[-3.0,56.0],[-3.0,57.0],[-4.0,57.0],[-4.0,56.0]]]}}
	]}`
	bs, err := scenario.ParseGeoJSON([]byte(doc), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 1 {
		t.Fatalf("got %d boundaries, want 1", len(bs))
	}
	// The ONS field name, which is what a real UK download actually uses.
	if bs[0].Name != "Scotland" {
		t.Errorf("name is %q; the ONS property was not recognised", bs[0].Name)
	}
}

// A file with nothing but points is a different problem from a file with a bad
// polygon, and silently returning an empty region would look like a boundary
// that contains nothing.
func TestAllPointsIsAnError(t *testing.T) {
	doc := `{"type":"FeatureCollection","features":[
		{"type":"Feature","properties":{},"geometry":{"type":"Point","coordinates":[-3.9,56.7]}}
	]}`
	_, err := scenario.ParseGeoJSON([]byte(doc), "")
	if err == nil {
		t.Fatal("a file with no polygons returned no error")
	}
	if !strings.Contains(err.Error(), "no polygons") {
		t.Errorf("error should say what was wrong: %v", err)
	}
}

// Sources hand out all three shapes and a user pasting a file should not have
// to know which they were given.
func TestAcceptsCollectionFeatureAndBareGeometry(t *testing.T) {
	poly := `{"type":"Polygon","coordinates":[[[-4.0,56.0],[-3.0,56.0],[-3.0,57.0],[-4.0,57.0],[-4.0,56.0]]]}`
	for name, doc := range map[string]string{
		"bare geometry":      poly,
		"feature":            `{"type":"Feature","properties":{},"geometry":` + poly + `}`,
		"feature collection": `{"type":"FeatureCollection","features":[{"type":"Feature","properties":{},"geometry":` + poly + `}]}`,
	} {
		bs, err := scenario.ParseGeoJSON([]byte(doc), "")
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(bs) != 1 {
			t.Errorf("%s: got %d boundaries", name, len(bs))
		}
	}
}

// Multi-select is the normal case (ADR-0019), so a region built from several
// files has to behave as one area.
func TestMultipleSelectionsUnion(t *testing.T) {
	a, err := scenario.ParseGeoJSON([]byte(`{"type":"Feature","properties":{"name":"Perthshire"},"geometry":{
		"type":"Polygon","coordinates":[[[-4.0,56.0],[-3.0,56.0],[-3.0,57.0],[-4.0,57.0],[-4.0,56.0]]]}}`), "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := scenario.ParseGeoJSON([]byte(`{"type":"Feature","properties":{"name":"Fife"},"geometry":{
		"type":"Polygon","coordinates":[[[-3.5,56.0],[-2.5,56.0],[-2.5,56.4],[-3.5,56.4],[-3.5,56.0]]]}}`), "")
	if err != nil {
		t.Fatal(err)
	}

	r := scenario.Region{Boundaries: append(a, b...), MarginKm: scenario.DefaultMarginKm}
	for _, p := range []scenario.LatLon{{Lat: 56.7, Lon: -3.9}, {Lat: 56.2, Lon: -2.8}} {
		if !r.Contains(p) {
			t.Errorf("%v is in one of the two selections but the union excluded it", p)
		}
	}
	// The margin is what stops a node just outside from being dropped, which
	// would produce a mesh that behaves better than reality.
	outside := scenario.LatLon{Lat: 55.95, Lon: -2.6}
	if r.Contains(outside) {
		t.Fatal("test point is not actually outside")
	}
	if !r.Participates(outside) {
		t.Error("a node just outside the boundary was excluded from the RF entirely")
	}
}
