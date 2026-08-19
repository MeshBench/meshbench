package basemap_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MeshBench/meshbench/internal/world/basemap"
)

func tile(c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

type server struct {
	body     []byte
	status   int
	requests int64
	agents   []string
	urls     []string
}

func (s *server) Do(req *http.Request) (*http.Response, error) {
	atomic.AddInt64(&s.requests, 1)
	s.agents = append(s.agents, req.Header.Get("User-Agent"))
	s.urls = append(s.urls, req.URL.String())
	code := s.status
	if code == 0 {
		code = 200
	}
	return &http.Response{
		StatusCode: code, Status: http.StatusText(code),
		Body: io.NopCloser(bytes.NewReader(s.body)),
	}, nil
}

func store(t *testing.T, srv *server) *basemap.Store {
	t.Helper()
	s, err := basemap.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.HTTP = srv
	return s
}

// Every one of these sources requires attribution, and a layer that ships
// without it is a licence breach rather than an untidy map.
func TestEveryLayerCarriesItsTerms(t *testing.T) {
	layers := basemap.Layers()
	if len(layers) < 3 {
		t.Fatalf("only %d layers", len(layers))
	}
	for _, l := range layers {
		if l.Attribution == "" {
			t.Errorf("%s has no attribution", l.ID)
		}
		if l.Terms == "" {
			t.Errorf("%s does not say what its terms are", l.ID)
		}
		if l.MaxZoom <= 0 {
			t.Errorf("%s has no zoom cap", l.ID)
		}
		if !strings.Contains(l.URL, "{z}") || !strings.Contains(l.URL, "{x}") ||
			!strings.Contains(l.URL, "{y}") {
			t.Errorf("%s URL is not a tile template: %s", l.ID, l.URL)
		}
	}
}

// ADR-0019's finding, applied here: naive zoom selection is what turns a pan
// into a several-hundred-tile download.
func TestZoomMatchesTheViewAndIsCapped(t *testing.T) {
	l, _ := basemap.ByID("osm")

	// At 94 m/px near 56 degrees north, the matching zoom is about 10.
	if z := basemap.ZoomFor(94, 56.7, l); z < 9 || z > 11 {
		t.Errorf("94 m/px gave zoom %d", z)
	}
	// Zooming in must ask for a deeper tile.
	if in, out := basemap.ZoomFor(5, 56.7, l), basemap.ZoomFor(500, 56.7, l); in <= out {
		t.Errorf("zoom did not follow the view: %d at 5 m/px, %d at 500 m/px", in, out)
	}
	// And it must never exceed what the source publishes.
	if z := basemap.ZoomFor(0.01, 56.7, l); z > l.MaxZoom {
		t.Errorf("asked for zoom %d beyond the source's %d", z, l.MaxZoom)
	}
}

// OpenStreetMap's policy requires an identifying User-Agent and blocks generic
// library defaults. Sending none is how an address gets banned.
func TestUserAgentIsSentAndRequired(t *testing.T) {
	srv := &server{body: tile(color.RGBA{R: 10, G: 120, B: 30, A: 255})}
	s := store(t, srv)
	l, _ := basemap.ByID("osm")

	if err := s.Fetch(context.Background(), l, 10, 500, 320); err != nil {
		t.Fatal(err)
	}
	if len(srv.agents) == 0 || !strings.Contains(srv.agents[0], "MeshcoreSim") {
		t.Errorf("User-Agent was %q", srv.agents)
	}

	s.UserAgent = ""
	if err := s.Fetch(context.Background(), l, 10, 501, 320); err == nil {
		t.Error("a fetch with no User-Agent was allowed")
	}
}

// Drawing must never wait on the network. A redraw that can block is a window
// that stops painting, which is indistinguishable from a crash.
func TestCachedNeverFetches(t *testing.T) {
	srv := &server{body: tile(color.RGBA{R: 10, G: 120, B: 30, A: 255})}
	s := store(t, srv)
	l, _ := basemap.ByID("osm")

	if _, ok := s.Cached(l, 10, 500, 320); ok {
		t.Error("an uncached tile was returned")
	}
	if n := atomic.LoadInt64(&srv.requests); n != 0 {
		t.Errorf("Cached issued %d requests", n)
	}

	if err := s.Fetch(context.Background(), l, 10, 500, 320); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Cached(l, 10, 500, 320); !ok {
		t.Error("a fetched tile was not then cached")
	}
}

// Sea has no imagery tiles at some zooms. Without remembering that, a map over
// water re-requests the same missing tiles on every redraw forever.
func TestMissingTilesAreRememberedNotRetried(t *testing.T) {
	srv := &server{status: 404}
	s := store(t, srv)
	l, _ := basemap.ByID("esri-imagery")

	for i := 0; i < 5; i++ {
		if err := s.Fetch(context.Background(), l, 10, 500, 320); err != nil {
			t.Fatalf("a 404 should not be an error: %v", err)
		}
	}
	if n := atomic.LoadInt64(&srv.requests); n != 1 {
		t.Errorf("a missing tile was requested %d times", n)
	}
}

// An error page must never land in the cache, because nothing evicts.
func TestBadResponsesAreNotCached(t *testing.T) {
	srv := &server{body: []byte("<html>rate limited</html>")}
	s := store(t, srv)
	l, _ := basemap.ByID("osm")

	if err := s.Fetch(context.Background(), l, 10, 500, 320); err == nil {
		t.Fatal("an HTML page was accepted as a tile")
	}
	if _, ok := s.Cached(l, 10, 500, 320); ok {
		t.Error("the error page was cached")
	}
}

// Esri serves {z}/{y}/{x}; OSM serves {z}/{x}/{y}. Swapping them produces
// perfectly valid tiles of somewhere else entirely.
func TestTemplateOrderIsHonoured(t *testing.T) {
	srv := &server{body: tile(color.RGBA{R: 1, G: 2, B: 3, A: 255})}
	s := store(t, srv)

	esri, _ := basemap.ByID("esri-imagery")
	if err := s.Fetch(context.Background(), esri, 7, 63, 40); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(srv.urls[0], "/7/40/63") {
		t.Errorf("Esri URL is %s; it takes z/y/x", srv.urls[0])
	}

	osm, _ := basemap.ByID("osm")
	if err := s.Fetch(context.Background(), osm, 7, 63, 40); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(srv.urls[1], "/7/63/40.png") {
		t.Errorf("OSM URL is %s; it takes z/x/y", srv.urls[1])
	}
}

func TestEstimateCountsBeforeFetching(t *testing.T) {
	srv := &server{body: tile(color.RGBA{R: 10, G: 120, B: 30, A: 255})}
	s := store(t, srv)
	l, _ := basemap.ByID("osm")

	e := s.Estimate(l, 56.6, 56.8, -4.0, -3.7, 11)
	if e.Tiles < 2 || e.ToFetch != e.Tiles {
		t.Fatalf("estimate over an empty cache: %+v", e)
	}
	if e.BytesRough <= 0 {
		t.Error("no size estimate")
	}

	var last, total int
	if err := s.Prefetch(context.Background(), l, 56.6, 56.8, -4.0, -3.7, 11,
		func(d, tot int) { last, total = d, tot }); err != nil {
		t.Fatal(err)
	}
	if total == 0 || last != total {
		t.Errorf("progress ended at %d/%d", last, total)
	}
	if after := s.Estimate(l, 56.6, 56.8, -4.0, -3.7, 11); after.ToFetch != 0 {
		t.Errorf("%d tiles still to fetch after a prefetch", after.ToFetch)
	}
}

func TestPixelAtReadsTheTile(t *testing.T) {
	want := color.RGBA{R: 12, G: 200, B: 90, A: 255}
	srv := &server{body: tile(want)}
	s := store(t, srv)
	l, _ := basemap.ByID("osm")

	const lat, lon, zoom = 56.7, -3.9, 11
	if _, _, _, _, ok := s.PixelAt(l, lat, lon, zoom); ok {
		t.Error("an uncached tile returned a pixel")
	}
	for _, xy := range basemap.TilesFor(lat-0.01, lat+0.01, lon-0.01, lon+0.01, zoom) {
		if err := s.Fetch(context.Background(), l, zoom, xy[0], xy[1]); err != nil {
			t.Fatal(err)
		}
	}
	r, g, b, a, ok := s.PixelAt(l, lat, lon, zoom)
	if !ok {
		t.Fatal("a cached tile did not yield a pixel")
	}
	if r != want.R || g != want.G || b != want.B || a != want.A {
		t.Errorf("got %d,%d,%d,%d want %v", r, g, b, a, want)
	}
}
