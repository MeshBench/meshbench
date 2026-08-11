package terrain

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

// Terrarium tiles: elevation encoded in RGB, from the AWS elevation-tiles-prod
// dataset. Height in metres is (R*256 + G + B/256) - 32768, so one blue step is
// about 4 mm and the encoding is effectively continuous.
//
// The dataset is derived from several national sources with their own
// attribution terms, and that attribution has to appear in the UI. Same
// obligation hamreach carries; the same rule applies here.
const (
	DefaultTileURL = "https://s3.amazonaws.com/elevation-tiles-prod/terrarium/{z}/{x}/{y}.png"
	// DefaultZoom is about 30 m per pixel at UK latitudes, which matches the
	// underlying data. Asking for more zoom does not add detail, it upsamples.
	DefaultZoom = 12
	tileSize    = 256
)

// Doer is the slice of http.Client used here, so a test can answer without a
// network and a caller can wrap retries around it.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// TileStore fetches and caches terrarium tiles.
//
// Cached on disk, permanently. Elevation does not change, and a scenario run
// twice should not download hundreds of megabytes twice — nor should a second
// user of the same area. Nothing here ever evicts: a cache that quietly drops
// tiles turns a reproducible run into an occasionally-different one.
type TileStore struct {
	// CacheDir holds downloaded tiles. Required.
	CacheDir string
	// URL template with {z}/{x}/{y}. Empty means DefaultTileURL.
	URL  string
	Zoom int
	HTTP Doer

	// Offline refuses to download and serves only what is cached. The point of
	// having it is that a field laptop should fail loudly rather than block for
	// thirty seconds per tile on a tethered phone.
	Offline bool

	// OnProgress, if set, is called as tiles are fetched.
	OnProgress func(done, total int)

	mu     sync.RWMutex
	loaded map[string]*tile
	// inflight prevents the same tile being fetched by several goroutines,
	// which on a raster computation is the normal case rather than a rare one.
	inflight map[string]*sync.WaitGroup
}

type tile struct {
	heights []float32 // tileSize*tileSize, metres
}

// NewTileStore prepares a store, creating the cache directory.
func NewTileStore(cacheDir string) (*TileStore, error) {
	if cacheDir == "" {
		return nil, fmt.Errorf("terrain: tile store needs a cache directory")
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("terrain: cache directory: %w", err)
	}
	return &TileStore{
		CacheDir: cacheDir,
		Zoom:     DefaultZoom,
		loaded:   map[string]*tile{},
		inflight: map[string]*sync.WaitGroup{},
	}, nil
}

func (s *TileStore) zoom() int {
	if s.Zoom <= 0 {
		return DefaultZoom
	}
	return s.Zoom
}

// TileXY is the slippy-map tile containing a coordinate at a zoom level.
func TileXY(lat, lon float64, zoom int) (x, y int) {
	n := math.Exp2(float64(zoom))
	x = int(math.Floor((lon + 180) / 360 * n))
	latRad := lat * math.Pi / 180
	y = int(math.Floor((1 - math.Log(math.Tan(latRad)+1/math.Cos(latRad))/math.Pi) / 2 * n))
	return x, y
}

// TilesForBounds counts the tiles a bounding box needs, which is what an
// estimate is made of: a user asked to wait should be told for how long and for
// how much, before it starts rather than after.
func TilesForBounds(south, north, west, east float64, zoom int) (count int, x0, y0, x1, y1 int) {
	x0, y1 = TileXY(south, west, zoom)
	x1, y0 = TileXY(north, east, zoom)
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	return (x1 - x0 + 1) * (y1 - y0 + 1), x0, y0, x1, y1
}

// Estimate describes what covering a box will cost.
type Estimate struct {
	Tiles      int
	Cached     int
	ToFetch    int
	BytesRough int64
}

// Estimate reports the download before it happens.
//
// The average terrarium tile is around 60 kB; the figure is approximate and
// says so in its name. It exists so a UI can say "412 tiles, about 25 MB" and
// let someone on a tethered connection decline.
func (s *TileStore) Estimate(south, north, west, east float64) Estimate {
	count, x0, y0, x1, y1 := TilesForBounds(south, north, west, east, s.zoom())
	e := Estimate{Tiles: count}
	for x := x0; x <= x1; x++ {
		for y := y0; y <= y1; y++ {
			if _, err := os.Stat(s.path(x, y)); err == nil {
				e.Cached++
			}
		}
	}
	e.ToFetch = e.Tiles - e.Cached
	const roughBytesPerTile = 60 << 10
	e.BytesRough = int64(e.ToFetch) * roughBytesPerTile
	return e
}

// Prefetch downloads everything a box needs, reporting progress.
//
// Done up front rather than lazily during a computation because a raster that
// stalls for a minute in the middle looks like a hang, and because the total is
// only knowable in advance.
func (s *TileStore) Prefetch(ctx context.Context, south, north, west, east float64) error {
	count, x0, y0, x1, y1 := TilesForBounds(south, north, west, east, s.zoom())
	done := 0
	for x := x0; x <= x1; x++ {
		for y := y0; y <= y1; y++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			if _, err := s.get(ctx, x, y); err != nil {
				return err
			}
			done++
			if s.OnProgress != nil {
				s.OnProgress(done, count)
			}
		}
	}
	return nil
}

func (s *TileStore) path(x, y int) string {
	return filepath.Join(s.CacheDir, fmt.Sprintf("%d", s.zoom()), fmt.Sprintf("%d", x), fmt.Sprintf("%d.png", y))
}

func (s *TileStore) key(x, y int) string { return fmt.Sprintf("%d/%d/%d", s.zoom(), x, y) }

func (s *TileStore) get(ctx context.Context, x, y int) (*tile, error) {
	k := s.key(x, y)

	s.mu.RLock()
	t, ok := s.loaded[k]
	wg := s.inflight[k]
	s.mu.RUnlock()
	if ok {
		return t, nil
	}
	if wg != nil {
		// Another goroutine is already fetching this tile. A raster asks for
		// the same tile from thousands of cells, so without this the first
		// computation over a new area issues thousands of identical requests.
		wg.Wait()
		s.mu.RLock()
		t, ok = s.loaded[k]
		s.mu.RUnlock()
		if ok {
			return t, nil
		}
		return nil, fmt.Errorf("terrain: tile %s failed to load", k)
	}

	wg = &sync.WaitGroup{}
	wg.Add(1)
	s.mu.Lock()
	if existing, racing := s.inflight[k]; racing {
		s.mu.Unlock()
		existing.Wait()
		return s.get(ctx, x, y)
	}
	s.inflight[k] = wg
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.inflight, k)
		s.mu.Unlock()
		wg.Done()
	}()

	loaded, err := s.load(ctx, x, y)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.loaded[k] = loaded
	s.mu.Unlock()
	return loaded, nil
}

func (s *TileStore) load(ctx context.Context, x, y int) (*tile, error) {
	p := s.path(x, y)
	if b, err := os.ReadFile(p); err == nil {
		return decodeTerrarium(b)
	}
	if s.Offline {
		return nil, fmt.Errorf("terrain: tile %s is not cached and downloads are off", s.key(x, y))
	}

	url := s.URL
	if url == "" {
		url = DefaultTileURL
	}
	url = replaceAll(url, s.zoom(), x, y)

	client := s.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("terrain: build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("terrain: fetch %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("terrain: %s returned %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("terrain: read %s: %w", url, err)
	}

	t, err := decodeTerrarium(body)
	if err != nil {
		return nil, fmt.Errorf("terrain: %s: %w", url, err)
	}
	// Written after decoding, so a truncated or HTML response never lands in
	// the cache. A poisoned cache entry is permanent by design here.
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err == nil {
		_ = os.WriteFile(p, body, 0o644)
	}
	return t, nil
}

func replaceAll(url string, z, x, y int) string {
	out := ""
	for i := 0; i < len(url); i++ {
		if url[i] == '{' {
			switch {
			case len(url) >= i+3 && url[i:i+3] == "{z}":
				out += fmt.Sprint(z)
				i += 2
				continue
			case len(url) >= i+3 && url[i:i+3] == "{x}":
				out += fmt.Sprint(x)
				i += 2
				continue
			case len(url) >= i+3 && url[i:i+3] == "{y}":
				out += fmt.Sprint(y)
				i += 2
				continue
			}
		}
		out += string(url[i])
	}
	return out
}

func decodeTerrarium(b []byte) (*tile, error) {
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("not a PNG tile: %w", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != tileSize || bounds.Dy() != tileSize {
		return nil, fmt.Errorf("tile is %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), tileSize, tileSize)
	}
	t := &tile{heights: make([]float32, tileSize*tileSize)}
	for y := 0; y < tileSize; y++ {
		for x := 0; x < tileSize; x++ {
			r, g, bl, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			// RGBA() returns 16-bit; terrarium is 8-bit per channel.
			t.heights[y*tileSize+x] = float32(float64(r>>8)*256 + float64(g>>8) + float64(bl>>8)/256 - 32768)
		}
	}
	return t, nil
}
