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
	"sync/atomic"
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

	// MaxLoadedTiles caps the decoded tiles held in memory. Zero means the
	// default.
	//
	// A decoded tile is 256x256 float32, a quarter of a megabyte, and this
	// cache had no bound at all: a workbench left running over a national
	// scenario reached 9.5 GB, of which 1.5 GB was this map after twenty-five
	// seconds. The tiles on disk number 32,254, so it was going further.
	MaxLoadedTiles int

	mu     sync.RWMutex
	loaded map[string]*tile
	// order is insertion order, for eviction. A ring of keys rather than a
	// real LRU: profile sampling walks a line across the map and comes back
	// along a neighbouring one, so what is worth keeping is what was read
	// recently in time, which insertion order already approximates.
	order []string
	// inflight prevents the same tile being fetched by several goroutines,
	// which on a raster computation is the normal case rather than a rare one.
	inflight map[string]*sync.WaitGroup

	// fetched counts the bytes that have actually crossed the network, as
	// opposed to the tiles that have. A tile count is a fine percentage and a
	// poor answer to the only question somebody on a hotspot is asking, which
	// is how much of their allowance this has spent. Atomic because the
	// fetchers run eight at a time.
	fetched atomic.Int64
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
	e.BytesRough = int64(e.ToFetch) * roughBytesPerTile
	return e
}

// roughBytesPerTile is the average terrarium tile, near enough to price a
// download before it happens and never near enough to report as a measurement.
//
// Measured rather than remembered, and it errs high on purpose. The figure
// here was 60 kB, which quoted 365 MB for the ground under the Scotland and
// Ireland network; 6,233 tiles later that cache held 525 MB, an average of
// 82 kB. A quote somebody decides on has to be wrong in the direction that
// costs them nothing: an estimate that flatters is how a metered connection
// agrees to half again as much as it was told.
const roughBytesPerTile = 82 << 10

// EstimateTiles prices an explicit tile list, the shape PrefetchTiles takes.
//
// The list form exists for the same reason PrefetchTiles does: the tiles under
// a network's links are a fraction of the box around it, so pricing the box
// quotes a figure several times the real one and would have somebody decline a
// download they could easily afford.
func (s *TileStore) EstimateTiles(tiles [][2]int) Estimate {
	e := Estimate{Tiles: len(tiles)}
	for _, t := range tiles {
		if _, err := os.Stat(s.path(t[0], t[1])); err == nil {
			e.Cached++
		}
	}
	e.ToFetch = e.Tiles - e.Cached
	e.BytesRough = int64(e.ToFetch) * roughBytesPerTile
	return e
}

// Prefetch downloads what a box is missing, reporting progress over exactly
// that: the tiles it will actually move.
//
// Done up front rather than lazily during a computation because a raster that
// stalls for a minute in the middle looks like a hang, and because the total is
// only knowable in advance. Two properties are deliberate. Cached tiles are
// skipped by a stat, never by get(): getting a cached tile decodes it into
// memory, so a prefetch over an already-cached country cost gigabytes of RAM
// to confirm what the filesystem already knew. And the missing ones download
// on several connections at once - one at a time, a few thousand tiles of a
// fresh region is most of an hour spent watching one request's latency.
func (s *TileStore) Prefetch(ctx context.Context, south, north, west, east float64) error {
	_, x0, y0, x1, y1 := TilesForBounds(south, north, west, east, s.zoom())
	tiles := make([][2]int, 0, (x1-x0+1)*(y1-y0+1))
	for x := x0; x <= x1; x++ {
		for y := y0; y <= y1; y++ {
			tiles = append(tiles, [2]int{x, y})
		}
	}
	return s.PrefetchTiles(ctx, tiles)
}

// PrefetchTiles downloads whichever of the given tiles are not on disk yet.
//
// The list form exists because a bounding box is usually the wrong shape: a
// link warm samples terrain only under the lines between its nodes, and the
// rectangle around a coastal country is mostly open sea those lines never
// cross. Prefetching the box fetched twenty-eight thousand tiles of Atlantic
// while the operator watched the warm sit on zero percent.
func (s *TileStore) PrefetchTiles(ctx context.Context, tiles [][2]int) error {
	var missing [][2]int
	for _, t := range tiles {
		if _, err := os.Stat(s.path(t[0], t[1])); err != nil {
			missing = append(missing, t)
		}
	}
	if s.OnProgress != nil {
		s.OnProgress(0, len(missing))
	}
	if len(missing) == 0 {
		return nil
	}

	// Bounded fan-out: kind to the tile host, decisive against latency. The
	// first error stops the hand-out - a host that refused one tile is about
	// to refuse the rest, and a thousand further failures say nothing new.
	const fetchers = 8
	jobs := make(chan [2]int)
	var wg sync.WaitGroup
	// One voice for progress: the callback's contract predates the fan-out,
	// and every existing caller writes plain state from it. Serialised here,
	// the count also stays monotonic instead of arriving shuffled.
	var progressMu sync.Mutex
	done := 0
	var firstErr error
	var errOnce sync.Once
	fetchCtx, stop := context.WithCancel(ctx)
	defer stop()
	for w := 0; w < fetchers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range jobs {
				if fetchCtx.Err() != nil {
					continue
				}
				if _, err := s.get(fetchCtx, t[0], t[1]); err != nil {
					errOnce.Do(func() { firstErr = err; stop() })
					continue
				}
				progressMu.Lock()
				done++
				if s.OnProgress != nil {
					s.OnProgress(done, len(missing))
				}
				progressMu.Unlock()
			}
		}()
	}
	for _, t := range missing {
		if fetchCtx.Err() != nil {
			break
		}
		jobs <- t
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

func (s *TileStore) path(x, y int) string {
	s.mu.RLock()
	dir := s.CacheDir
	s.mu.RUnlock()
	return filepath.Join(dir, fmt.Sprintf("%d", s.zoom()), fmt.Sprintf("%d", x), fmt.Sprintf("%d.png", y))
}

// SetCacheDir points the store at a new directory, under the lock.
//
// For the Configuration page's cache move: called only after the tiles have
// been carried over, and the decoded tiles in memory survive it, so nothing
// re-decodes and nothing re-downloads.
func (s *TileStore) SetCacheDir(dir string) {
	s.mu.Lock()
	s.CacheDir = dir
	s.mu.Unlock()
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
	s.remember(k, loaded)
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
	// Counted here rather than after the decode: the bytes were spent whether
	// or not what arrived turns out to be a tile.
	s.fetched.Add(int64(len(body)))

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

// DefaultMaxLoadedTiles is 24,576 decoded tiles at a quarter megabyte each:
// about 6 GB - the whole of Britain and Ireland at zoom 12 with headroom,
// which is the largest thing anyone has imported. Settable from the
// Configuration page in either direction.
//
// Both edges of this number have been hit. The old cap of 1,024 believed its
// own comment - "enough that a profile across a country does not thrash" -
// and it was not: a national warm evicted constantly and re-decoded PNGs at
// milliseconds each, and the thrash was the whole cost of a nine-minute warm
// whose arithmetic took 30 milliseconds. The next cap of 40,960 fixed that
// by allowing 10 GB, and on a 31 GB desktop that was already running a
// browser and a compile it was the difference between simulating and
// swapping - the operator watched their RAM run out mid-warm. The working
// set is what decides: below it, milliseconds per sample; above it, wasted
// memory. UK-and-Ireland at zoom 12 is roughly 18,000 tiles.
const DefaultMaxLoadedTiles = 24576

// remember stores a decoded tile and evicts the oldest once the cap is passed.
// Called with the lock held.
func (s *TileStore) remember(k string, t *tile) {
	if _, exists := s.loaded[k]; !exists {
		s.order = append(s.order, k)
	}
	s.loaded[k] = t

	max := s.MaxLoadedTiles
	if max <= 0 {
		max = DefaultMaxLoadedTiles
	}
	for len(s.order) > max {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.loaded, oldest)
	}
}

// LoadedTiles is how many decoded tiles are held, for a test and for anybody
// wondering where the memory went.
func (s *TileStore) LoadedTiles() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.loaded)
}

// FetchedBytes is how much has crossed the network since this store was made.
//
// Measured, not estimated: Estimate multiplies a tile count by an average, and
// the number an operator is owed while a download is running is the one their
// connection has actually carried.
func (s *TileStore) FetchedBytes() int64 { return s.fetched.Load() }
