package terrain_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/A13xB0/meshcoresim/internal/terrain"
)

// terrariumTile renders a tile whose height varies linearly across it, so a
// sampler's interpolation can be checked against a value that can be stated
// rather than one read back out of the code.
func terrariumTile(heightAt func(x, y int) float64) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			v := heightAt(x, y) + 32768
			r := int(v / 256)
			g := int(v) % 256
			b := int((v - float64(int(v))) * 256)
			img.Set(x, y, color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

type tileServer struct {
	body     []byte
	requests int64
	status   int
	mu       sync.Mutex
	urls     []string
}

func (s *tileServer) Do(req *http.Request) (*http.Response, error) {
	atomic.AddInt64(&s.requests, 1)
	s.mu.Lock()
	s.urls = append(s.urls, req.URL.String())
	s.mu.Unlock()
	code := s.status
	if code == 0 {
		code = 200
	}
	return &http.Response{
		StatusCode: code,
		Status:     http.StatusText(code),
		Body:       io.NopCloser(bytes.NewReader(s.body)),
	}, nil
}

func store(t *testing.T, srv *tileServer) *terrain.TileStore {
	t.Helper()
	s, err := terrain.NewTileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.HTTP = srv
	s.URL = "http://tiles.test/{z}/{x}/{y}.png"
	return s
}

// The encoding is the one thing that must be exactly right: a factor or offset
// error puts all of Scotland underwater or in orbit, and the maps still look
// entirely plausible.
func TestTerrariumEncoding(t *testing.T) {
	srv := &tileServer{body: terrariumTile(func(_, _ int) float64 { return 1344 })}
	s := store(t, srv)

	h, ok := s.ElevationM(56.7967, -5.0036) // Ben Nevis, near enough
	if !ok {
		t.Fatal("no elevation returned")
	}
	if h < 1343 || h > 1345 {
		t.Errorf("decoded %.2f m from a tile encoding 1344 m", h)
	}
}

// Bilinear, not nearest-neighbour. A profile walks a tile in steps far smaller
// than a pixel, and nearest-neighbour turns a hillside into a staircase that
// the diffraction model then reads as a row of knife edges.
func TestSamplingIsInterpolated(t *testing.T) {
	srv := &tileServer{body: terrariumTile(func(x, _ int) float64 { return float64(x) * 10 })}
	s := store(t, srv)

	// Two points a fraction of a pixel apart must differ, and must not be equal
	// to either integer pixel value.
	const lat = 56.7
	var heights []float64
	for _, lon := range []float64{-3.90000, -3.89999, -3.89998} {
		h, ok := s.ElevationM(lat, lon)
		if !ok {
			t.Fatal("no elevation")
		}
		heights = append(heights, h)
	}
	if heights[0] == heights[1] && heights[1] == heights[2] {
		t.Errorf("three sub-pixel samples all returned %.4f — this is nearest-neighbour", heights[0])
	}
	for i := 1; i < len(heights); i++ {
		if heights[i] < heights[i-1] {
			t.Errorf("height fell across a monotonically rising tile: %v", heights)
		}
	}
}

// Outside the downloaded area is ignorance, not sea level. Answering zero draws
// confident coverage across water the DEM never described.
func TestOfflineMissesLoudly(t *testing.T) {
	s := store(t, &tileServer{body: terrariumTile(func(int, int) float64 { return 100 })})
	s.Offline = true
	if _, ok := s.ElevationM(56.7, -3.9); ok {
		t.Error("an uncached tile returned an elevation with downloads off")
	}
}

// A raster asks for the same tile from thousands of cells. Without
// deduplication the first computation over a new area issues thousands of
// identical requests, which is both slow and rude to the tile server.
func TestConcurrentSamplesFetchEachTileOnce(t *testing.T) {
	srv := &tileServer{body: terrariumTile(func(int, int) float64 { return 250 })}
	s := store(t, srv)

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := s.ElevationM(56.7, -3.9); !ok {
				t.Error("no elevation")
			}
		}()
	}
	wg.Wait()

	// Four tiles at most: a bilinear sample can straddle a tile corner.
	if n := atomic.LoadInt64(&srv.requests); n > 4 {
		t.Errorf("%d requests for at most 4 tiles", n)
	}
}

// A truncated or HTML response must never land in the cache. A poisoned entry
// is permanent here by design, because nothing evicts.
func TestBadResponsesAreNotCached(t *testing.T) {
	srv := &tileServer{body: []byte("<!doctype html><title>404</title>")}
	s := store(t, srv)

	if _, ok := s.ElevationM(56.7, -3.9); ok {
		t.Fatal("an HTML page was accepted as a tile")
	}
	// Nothing should have been written.
	matches, _ := filepath.Glob(filepath.Join(s.CacheDir, "*", "*", "*.png"))
	if len(matches) != 0 {
		t.Errorf("cached %d files from a bad response", len(matches))
	}
}

// An estimate exists so someone on a tethered connection can decline before the
// download starts rather than after.
func TestEstimateCountsBeforeFetching(t *testing.T) {
	s := store(t, &tileServer{body: terrariumTile(func(int, int) float64 { return 100 })})

	e := s.Estimate(56.6, 56.9, -4.2, -3.6)
	if e.Tiles < 2 {
		t.Errorf("a 0.3 x 0.6 degree box needs %d tiles, which cannot be right", e.Tiles)
	}
	if e.ToFetch != e.Tiles || e.Cached != 0 {
		t.Errorf("an empty cache reported %d already cached", e.Cached)
	}
	if e.BytesRough <= 0 {
		t.Error("no size estimate")
	}

	ctx := context.Background()
	if err := s.Prefetch(ctx, 56.6, 56.9, -4.2, -3.6); err != nil {
		t.Fatal(err)
	}
	after := s.Estimate(56.6, 56.9, -4.2, -3.6)
	if after.ToFetch != 0 {
		t.Errorf("%d tiles still to fetch after a prefetch", after.ToFetch)
	}
}

func TestPrefetchReportsProgress(t *testing.T) {
	s := store(t, &tileServer{body: terrariumTile(func(int, int) float64 { return 100 })})
	var last, total int
	s.OnProgress = func(done, tot int) { last, total = done, tot }
	if err := s.Prefetch(context.Background(), 56.6, 56.7, -4.0, -3.9); err != nil {
		t.Fatal(err)
	}
	if total == 0 || last != total {
		t.Errorf("progress ended at %d/%d", last, total)
	}
}

func TestURLTemplateIsSubstituted(t *testing.T) {
	srv := &tileServer{body: terrariumTile(func(int, int) float64 { return 100 })}
	s := store(t, srv)
	s.Zoom = 11
	if _, ok := s.ElevationM(56.7, -3.9); !ok {
		t.Fatal("no elevation")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.urls) == 0 || strings.Contains(srv.urls[0], "{") {
		t.Errorf("template not substituted: %v", srv.urls)
	}
	if !strings.Contains(srv.urls[0], "/11/") {
		t.Errorf("zoom not honoured: %s", srv.urls[0])
	}
}
