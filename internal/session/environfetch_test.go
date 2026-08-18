package session

import (
	"io"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/environ"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// The four level-1 quadkeys are the definition; everything longer is the
// same two bits a digit.
func TestQuadkeyMatchesBingDefinition(t *testing.T) {
	for _, c := range []struct {
		x, y int
		want string
	}{{0, 0, "0"}, {1, 0, "1"}, {0, 1, "2"}, {1, 1, "3"}} {
		if got := quadkey(c.x, c.y, 1); got != c.want {
			t.Fatalf("quadkey(%d,%d,1) = %q, want %q", c.x, c.y, got, c.want)
		}
	}
	if got := quadkey(3, 5, 3); len(got) != 3 {
		t.Fatalf("a zoom-3 quadkey must have 3 digits, got %q", got)
	}
}

func TestQuadkeysForCoverTheBox(t *testing.T) {
	keys := quadkeysFor(55.9, 56.1, -3.3, -3.1)
	if len(keys) == 0 {
		t.Fatal("a real box resolved to no quadkeys")
	}
	for _, k := range keys {
		if len(k) != 9 {
			t.Fatalf("quadkey %q is not level 9", k)
		}
	}
}

func TestEnvironBoxNeedsNodes(t *testing.T) {
	if _, _, _, _, err := environBox(nil); err == nil {
		t.Fatal("an empty network produced a box")
	}
	n := scenario.Node{Name: "a", Kind: scenario.SimpleRepeater}
	n.Position.Lat, n.Position.Lon = 56, -3
	s, no, w, e, err := environBox([]scenario.Node{n})
	if err != nil {
		t.Fatal(err)
	}
	if s >= 56 || no <= 56 || w >= -3 || e <= -3 {
		t.Fatalf("box [%f %f %f %f] has no margin", s, no, w, e)
	}
}

// The rewrite must keep the tags - heights and materials ride in them - and
// leave out anything that is not a polygonal way.
func TestOverpassRewriteKeepsTagsAndDropsPoints(t *testing.T) {
	in := `{"elements":[
	 {"type":"way","geometry":[{"lat":56.0,"lon":-3.0},{"lat":56.001,"lon":-3.0},
	  {"lat":56.001,"lon":-3.001},{"lat":56.0,"lon":-3.0}],
	  "tags":{"building":"house","building:levels":"2"}},
	 {"type":"node","tags":{"building":"yes"}},
	 {"type":"way","geometry":[{"lat":1,"lon":1}],"tags":{"building":"yes"}}
	]}`
	rd, n, err := overpassToNDJSON(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("kept %d features, want the one real polygon", n)
	}
	out, _ := io.ReadAll(rd)
	line := string(out)
	for _, want := range []string{`"building":"house"`, `"building:levels":"2"`, `"Polygon"`} {
		if !strings.Contains(line, want) {
			t.Fatalf("rewrite lost %s in %s", want, line)
		}
	}
}

func TestOverpassAreaCapFailsLoudly(t *testing.T) {
	// Roughly the whole of Scotland: far past what a live pull is fair for.
	if a := boxAreaKm2(54.6, 58.7, -7.5, -1.9); a <= overpassMaxKm2 {
		t.Fatalf("Scotland measures %.0f km2, which should exceed the cap", a)
	}
	var s Sim
	_, _, err := s.fetchEnviron("osm", 54.6, 58.7, -7.5, -1.9, func(int, int) {})
	if err == nil || !strings.Contains(err.Error(), "envgen") {
		t.Fatalf("an oversized pull must refuse and point at envgen, got %v", err)
	}
}

func TestFetchEnvironRefusesUnknownSource(t *testing.T) {
	var s Sim
	if _, _, err := s.fetchEnviron("zillow", 56, 56.1, -3.1, -3, func(int, int) {}); err == nil {
		t.Fatal("an unknown source must refuse")
	}
}

// hasTiles must recognise what IngestGeoJSON actually writes - the cache
// check and the tile layout drifting apart means every pull downloads again.
func TestHasTilesMatchesIngestLayout(t *testing.T) {
	dir := t.TempDir()
	if hasTiles(dir) {
		t.Fatal("an empty directory claimed tiles")
	}
	one := `{"type":"Feature","geometry":{"type":"Polygon","coordinates":` +
		`[[[-3.0,56.0],[-3.0,56.0005],[-3.0005,56.0005],[-3.0,56.0]]]},` +
		`"properties":{"building":"house"}}`
	if _, err := environ.IngestGeoJSON(strings.NewReader(one), dir, "uk"); err != nil {
		t.Fatal(err)
	}
	if !hasTiles(dir) {
		t.Fatal("hasTiles cannot see what IngestGeoJSON wrote")
	}
}

func TestMergedSourceHonoursTheOverpassCap(t *testing.T) {
	var s Sim
	_, _, err := s.fetchEnviron("merged", 54.6, 58.7, -7.5, -1.9, func(int, int) {})
	if err == nil || !strings.Contains(err.Error(), "envgen") {
		t.Fatalf("an oversized merged pull must refuse and point at envgen, got %v", err)
	}
}
