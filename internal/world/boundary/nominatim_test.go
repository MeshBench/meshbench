package boundary_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/world/boundary"
)

// A square, as Nominatim would return one.
const square = `{"type":"Polygon","coordinates":[[[-3.0,56.0],[-3.0,56.1],[-2.9,56.1],[-2.9,56.0],[-3.0,56.0]]]}`

// serve answers every request with one body, and records what was asked for.
func serve(t *testing.T, status int, body string) (*boundary.Client, *string) {
	t.Helper()
	var asked string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.String()
		if ua := r.Header.Get("User-Agent"); ua == "" {
			t.Error("Nominatim blocks anonymous requests, so one must always be sent")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &boundary.Client{BaseURL: srv.URL, HTTP: srv.Client(), CacheDir: t.TempDir()}, &asked
}

func TestSearchReturnsPlacesThatHaveAnArea(t *testing.T) {
	c, asked := serve(t, http.StatusOK, `[
		{"name":"Fife","display_name":"Fife, Scotland","type":"administrative","geojson":`+square+`,
		 "lat":"56.05","lon":"-2.95"}]`)

	found, err := c.Search(context.Background(), "Fife")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("wanted one result, got %d", len(found))
	}
	f := found[0]
	if f.Name != "Fife" || f.DisplayName != "Fife, Scotland" || f.Kind != "administrative" {
		t.Errorf("the result did not carry its own description: %+v", f)
	}
	if len(f.Boundaries) == 0 {
		t.Fatal("a place with an area must come back with its outline")
	}
	if f.Boundaries[0].Name != "Fife" || f.Boundaries[0].Source != "osm" {
		t.Errorf("the outline should be named and attributed: %+v", f.Boundaries[0])
	}
	if f.Lat == 0 || f.Lon == 0 {
		t.Errorf("the point did not survive: %v, %v", f.Lat, f.Lon)
	}
	if !strings.Contains(*asked, "polygon_geojson=1") {
		t.Errorf("the search must ask for polygons, or nothing has an area: %s", *asked)
	}
}

// A search for a town also matches bus stops. Offering a point as a boundary
// would let somebody filter a network down to nothing, so results without an
// area are dropped rather than returned.
func TestSearchDropsResultsWithNoArea(t *testing.T) {
	c, _ := serve(t, http.StatusOK, `[
		{"name":"Perth bus stop","type":"bus_stop",
		 "geojson":{"type":"Point","coordinates":[-3.43,56.39]},"lat":"56.39","lon":"-3.43"},
		{"name":"Perth","type":"administrative","geojson":`+square+`,"lat":"56.4","lon":"-3.4"}]`)

	found, err := c.Search(context.Background(), "Perth")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Name != "Perth" {
		t.Fatalf("only the place with an area should come back, got %+v", found)
	}
}

// A search that matches only points is a failure with a reason, not an empty
// success that reads as "there is no such place".
func TestSearchSaysWhenNothingHasAnArea(t *testing.T) {
	c, _ := serve(t, http.StatusOK,
		`[{"name":"A stop","type":"bus_stop","geojson":{"type":"Point","coordinates":[0,0]}}]`)

	_, err := c.Search(context.Background(), "A stop")
	if err == nil {
		t.Fatal("wanted an error when nothing usable matched")
	}
	if !strings.Contains(err.Error(), "A stop") {
		t.Errorf("the error should name what was searched for: %v", err)
	}
}

func TestSearchReportsATransportFailure(t *testing.T) {
	c, _ := serve(t, http.StatusTooManyRequests, "slow down")
	if _, err := c.Search(context.Background(), "Fife"); err == nil {
		t.Fatal("a non-200 must be an error, not an empty result")
	}
}

func TestSearchReportsAnUnparseableAnswer(t *testing.T) {
	c, _ := serve(t, http.StatusOK, "{not json")
	if _, err := c.Search(context.Background(), "Fife"); err == nil {
		t.Fatal("a malformed answer must be an error")
	}
}

func TestSearchHonoursACancelledContext(t *testing.T) {
	c, _ := serve(t, http.StatusOK, `[]`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Search(ctx, "Fife"); err == nil {
		t.Fatal("a cancelled context must stop the search")
	}
}

// A found place is cached, so it can be used again with no network.
func TestAFoundPlaceIsUsableOffline(t *testing.T) {
	c, _ := serve(t, http.StatusOK,
		`[{"name":"Fife","type":"administrative","geojson":`+square+`,"lat":"56","lon":"-3"}]`)

	if _, err := c.Search(context.Background(), "Fife"); err != nil {
		t.Fatal(err)
	}
	offline := &boundary.Client{CacheDir: c.CacheDir}
	bounds, ok := offline.Cached("Fife")
	if !ok {
		t.Fatal("a place that was found should be readable again without the network")
	}
	if len(bounds) == 0 || bounds[0].Name != "Fife" || bounds[0].Source != "osm" {
		t.Errorf("the cached outline lost its identity: %+v", bounds)
	}
}

func TestCachedIsAbsentRatherThanWrong(t *testing.T) {
	dir := t.TempDir()
	c := &boundary.Client{CacheDir: dir}
	if _, ok := c.Cached("never asked for"); ok {
		t.Error("a place never fetched must not be reported as cached")
	}
	// A cache file that is not GeoJSON is a miss, not a parse failure that
	// propagates as an empty boundary.
	if err := os.WriteFile(filepath.Join(dir, "broken.geojson"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Cached("broken"); ok {
		t.Error("an unreadable cache entry must be a miss")
	}
	// With no cache directory there is nothing to read and nothing to panic on.
	if _, ok := (&boundary.Client{}).Cached("Fife"); ok {
		t.Error("a client with no cache directory has nothing cached")
	}
}

// The cache filename is derived from a place name that came off the network, so
// it must not be able to escape the cache directory or collide by case.
func TestACachedNameCannotEscapeTheCacheDirectory(t *testing.T) {
	dir := t.TempDir()
	c, _ := serve(t, http.StatusOK,
		`[{"name":"../../etc/passwd","type":"administrative","geojson":`+square+`,"lat":"0","lon":"0"}]`)
	c.CacheDir = dir

	if _, err := c.Search(context.Background(), "anything"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one cache file, got %d", len(entries))
	}
	name := entries[0].Name()
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		t.Fatalf("the cache filename can leave its directory: %q", name)
	}
	for _, r := range strings.TrimSuffix(name, ".geojson") {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
		if !valid {
			t.Fatalf("unexpected character %q in cache filename %q", r, name)
		}
	}
}

func TestReverseSearchAsksAtAStudyAreaZoom(t *testing.T) {
	c, asked := serve(t, http.StatusOK,
		`{"name":"Scotland","type":"administrative","geojson":`+square+`,"lat":"56","lon":"-3"}`)

	found, err := c.ReverseSearch(context.Background(), 56.05, -2.95)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) == 0 || found[0].Name != "Scotland" {
		t.Fatalf("wanted the containing area, got %+v", found)
	}
	// A finer zoom returns a suburb, and a boundary drawn round a suburb
	// excludes the network it was meant to hold.
	if !strings.Contains(*asked, "zoom=5") {
		t.Errorf("the reverse lookup must ask at region zoom: %s", *asked)
	}
}
