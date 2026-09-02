package environ

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	worldenv "github.com/MeshBench/meshbench/internal/rf/environ"
	"github.com/MeshBench/meshbench/internal/world/scenario"
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

// Patches are the pull's whole point: two nodes in the same town share one,
// two towns get one each, and a national network's empty middle is never
// asked for.
func TestEnvironPatchesMergeTownsAndSkipTheEmptyMiddle(t *testing.T) {
	if _, err := environPatches(nil); err == nil {
		t.Fatal("an empty network produced patches")
	}
	mk := func(name string, lat, lon float64) scenario.Node {
		n := scenario.Node{Name: name, Kind: scenario.SimpleRepeater}
		n.Position.Lat, n.Position.Lon = lat, lon
		return n
	}
	patches, err := environPatches([]scenario.Node{
		mk("a", 56.000, -3.000), mk("b", 56.010, -3.010), // one town
		mk("c", 57.500, -5.500), // another, far away
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(patches) != 2 {
		t.Fatalf("got %d patches, want the town merged into 1 plus 1 remote", len(patches))
	}
	// A country-spanning pair must not cost a country: the patches together
	// stay near 2 nodes' worth of ground, not the bounding box between them.
	if a := patchesAreaKm2(patches); a > 100 {
		t.Fatalf("3 nodes priced at %.0f km2; patches are meant to be local", a)
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
	rd, n, err := overpassToNDJSON(strings.NewReader(in), map[int64]bool{})
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
	if a := patchesAreaKm2(scotland()); a <= overpassMaxKm2 {
		t.Fatalf("Scotland measures %.0f km2, which should exceed the cap", a)
	}
	var s session.Sim
	_, _, err := fetchEnviron(&s, context.Background(), "osm", scotland(), func(int, int) {})
	// The route it names has to be one a downloaded build can take.
	// tools/envgen was not: it is a source tool and no release bundle
	// carries it, so the advice was closed to exactly the people who hit
	// the cap.
	if err == nil || !strings.Contains(err.Error(), "Microsoft alone") {
		t.Fatalf("an oversized pull must refuse and name a route that ships, got %v", err)
	}
}

// scotland is an oversized patch set - one box the size of the country.
func scotland() []llBox {
	return []llBox{{South: 54.6, North: 58.7, West: -7.5, East: -1.9}}
}

func TestFetchEnvironRefusesUnknownSource(t *testing.T) {
	var s session.Sim
	if _, _, err := fetchEnviron(&s, context.Background(), "zillow", []llBox{{South: 56, North: 56.1, West: -3.1, East: -3}}, func(int, int) {}); err == nil {
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
	if _, err := worldenv.IngestGeoJSON(strings.NewReader(one), dir, "uk"); err != nil {
		t.Fatal(err)
	}
	if !hasTiles(dir) {
		t.Fatal("hasTiles cannot see what IngestGeoJSON wrote")
	}
}

func TestMergedSourceHonoursTheOverpassCap(t *testing.T) {
	var s session.Sim
	_, _, err := fetchEnviron(&s, context.Background(), "merged", scotland(), func(int, int) {})
	if err == nil || !strings.Contains(err.Error(), "Microsoft alone") {
		t.Fatalf("an oversized merged pull must refuse and name a route that ships, got %v", err)
	}
}

// A way arriving in two chunks is one building, not two walls to pay for.
func TestOverpassDedupesAcrossChunks(t *testing.T) {
	way := `{"elements":[{"type":"way","id":42,"geometry":[{"lat":56.0,"lon":-3.0},
	 {"lat":56.001,"lon":-3.0},{"lat":56.001,"lon":-3.001},{"lat":56.0,"lon":-3.0}],
	 "tags":{"building":"yes"}}]}`
	seen := map[int64]bool{}
	if _, n, err := overpassToNDJSON(strings.NewReader(way), seen); err != nil || n != 1 {
		t.Fatalf("first chunk: n=%d err=%v, want the way kept", n, err)
	}
	if _, n, err := overpassToNDJSON(strings.NewReader(way), seen); err != nil || n != 0 {
		t.Fatalf("second chunk: n=%d err=%v, want the duplicate dropped", n, err)
	}
}

// The filter is what makes a 92-file national pull affordable: everything
// outside the patches is dropped on the way past, and what is kept arrives
// byte-for-byte intact for the ingester.
func TestFilterNDJSONKeepsOnlyThePatches(t *testing.T) {
	idx := newPatchIndex([]llBox{{South: 56.0, North: 56.1, West: -3.1, East: -3.0}})
	in := `{"type":"Feature","geometry":{"type":"Polygon","coordinates":[[[-3.05,56.05],[-3.05,56.051],[-3.051,56.05],[-3.05,56.05]]]},"properties":{"height":7.5}}
{"type":"Feature","geometry":{"type":"Polygon","coordinates":[[[-6.0,52.0],[-6.0,52.001],[-6.001,52.0],[-6.0,52.0]]]},"properties":{"height":3.0}}
not json at all
`
	var out strings.Builder
	kept, err := filterNDJSON(strings.NewReader(in), idx, &out)
	if err != nil {
		t.Fatal(err)
	}
	if kept != 1 {
		t.Fatalf("kept %d features, want only the one inside the patch", kept)
	}
	if !strings.Contains(out.String(), `"height":7.5`) || strings.Contains(out.String(), `"height":3.0`) {
		t.Fatalf("filter kept the wrong feature: %s", out.String())
	}
}

func TestPatchIndexAgreesWithTheBoxes(t *testing.T) {
	idx := newPatchIndex([]llBox{
		{South: 56.0, North: 56.1, West: -3.1, East: -3.0},
		{South: -1.0, North: 1.0, West: -1.0, East: 1.0}, // straddles the origin
	})
	for _, c := range []struct {
		lat, lon float64
		want     bool
	}{
		{56.05, -3.05, true}, {56.05, -2.95, false}, {55.95, -3.05, false},
		{0, 0, true}, {-0.9, 0.9, true}, {1.1, 0, false},
	} {
		if got := idx.contains(c.lat, c.lon); got != c.want {
			t.Fatalf("contains(%f,%f) = %v, want %v", c.lat, c.lon, got, c.want)
		}
	}
}
