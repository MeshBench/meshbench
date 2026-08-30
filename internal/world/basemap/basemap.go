// Package basemap draws the world under the simulation.
//
// Hillshaded terrain answers "what shape is the ground". It does not answer
// "is that a forest or a field", "is there a road to this site", or "what is
// that building on the ridge" — and those decide whether a site is reachable,
// buildable and legal long before the link budget does.
//
// Everything here follows ADR-0019's rules for terrain, because they are the
// right rules for any tiled download: estimate before fetching, fetch in the
// background, cache persistently, cap the zoom, and degrade honestly. A tile
// that is missing is drawn as missing. It is never drawn as empty ground.
//
// The licensing is not settled and this package does not pretend otherwise.
// Every layer carries its attribution and its terms, `RequiresReview` marks the
// ones that cannot be shipped without a decision, and the attribution string is
// not optional — see ADR-0021 and docs/shortcomings.md.
package basemap

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg" // Esri serves JPEG; OSM serves PNG.
	_ "image/png"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/MeshBench/meshbench/internal/diag"
)

// Store fetches and caches basemap tiles.
type Store struct {
	CacheDir string
	HTTP     interface {
		Do(*http.Request) (*http.Response, error)
	}

	// UserAgent identifies this application. OpenStreetMap's policy requires a
	// real one and blocks generic library defaults, so an empty value here is
	// a bug rather than a preference.
	UserAgent string

	// Offline serves only what is on disk.
	Offline bool

	mu     sync.RWMutex
	loaded map[string]*image.RGBA
	// missing remembers tiles the source does not have, so a blank area is not
	// re-requested on every redraw. Sea has no imagery tiles at some zooms and
	// a map over water would otherwise hammer the server forever.
	missing map[string]bool
}

func NewStore(cacheDir string) (*Store, error) {
	if cacheDir == "" {
		return nil, fmt.Errorf("basemap: needs a cache directory")
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("basemap: cache directory: %w", err)
	}
	return &Store{
		CacheDir:  cacheDir,
		UserAgent: "MeshcoreSim/0.1 (+https://github.com/MeshBench/meshbench)",
		loaded:    map[string]*image.RGBA{},
		missing:   map[string]bool{},
	}, nil
}

func key(l Layer, z, x, y int) string { return fmt.Sprintf("%s/%d/%d/%d", l.ID, z, x, y) }

// tileURL is the request for one tile. CARTO's raster service wants an API
// key on the URL; without one it serves an "API KEY REQUIRED" watermark tile
// that caches like a real answer.
func tileURL(l Layer, z, x, y int) string {
	url := expand(l.URL, z, x, y)
	if strings.Contains(url, "cartocdn.com") {
		if k := CartoKey(); k != "" {
			url += "?key=" + k
		}
	}
	return url
}

func (s *Store) path(l Layer, z, x, y int) string {
	return filepath.Join(s.CacheDir, l.ID, fmt.Sprint(z), fmt.Sprint(x), fmt.Sprint(y)+".img")
}

// tileName identifies a tile in a message, without the request URL.
//
// The URL carries the CARTO key as a query parameter, and these messages reach
// a terminal, a shell history and whatever somebody pastes into a bug report.
// The layer and the coordinates say which tile failed, which is the whole of
// what a reader needs.
func tileName(l Layer, z, x, y int) string {
	return fmt.Sprintf("%s tile %d/%d/%d", l.ID, z, x, y)
}

// causeOf strips the request URL out of a transport failure.
//
// net/http reports one as a *url.Error, which prints the whole URL it was
// given. Naming the tile and then wrapping that puts the key back into the
// message it was just taken out of, so the underlying cause is what travels.
func causeOf(err error) error {
	var u *url.Error
	if errors.As(err, &u) && u.Err != nil {
		return u.Err
	}
	return err
}

// Cached returns a tile only from memory or disk, never the network.
//
// Drawing uses this. A redraw that can block on an HTTP request is a window
// that stops painting, which is indistinguishable from a crash.
func (s *Store) Cached(l Layer, z, x, y int) (*image.RGBA, bool) {
	k := key(l, z, x, y)
	s.mu.RLock()
	img, ok := s.loaded[k]
	gone := s.missing[k]
	s.mu.RUnlock()
	if ok {
		return img, true
	}
	if gone {
		return nil, false
	}

	b, err := os.ReadFile(s.path(l, z, x, y))
	if err != nil {
		return nil, false
	}
	decoded, err := decode(b)
	if err != nil {
		return nil, false
	}
	s.mu.Lock()
	s.loaded[k] = decoded
	s.mu.Unlock()
	return decoded, true
}

// Fetch downloads a tile if it is not already cached.
func (s *Store) Fetch(ctx context.Context, l Layer, z, x, y int) error {
	if _, ok := s.Cached(l, z, x, y); ok {
		return nil
	}
	k := key(l, z, x, y)
	s.mu.RLock()
	gone := s.missing[k]
	s.mu.RUnlock()
	if gone {
		return nil
	}
	if s.Offline {
		return fmt.Errorf("basemap: %s is not cached and downloads are off", k)
	}
	if s.UserAgent == "" {
		return fmt.Errorf("basemap: no User-Agent; OpenStreetMap's policy requires one " +
			"and blocks generic defaults")
	}

	url := tileURL(l, z, x, y)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", s.UserAgent)

	client := s.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("basemap: fetch %s: %w", tileName(l, z, x, y), causeOf(err))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		// Genuinely absent, not a failure. Remembered so a map over water does
		// not re-request the same empty tiles on every redraw.
		s.mu.Lock()
		s.missing[k] = true
		s.mu.Unlock()
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("basemap: %s returned %s", tileName(l, z, x, y), resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	img, err := decode(body)
	if err != nil {
		return fmt.Errorf("basemap: %s: %w", tileName(l, z, x, y), err)
	}

	// Decoded before it is written, so an error page never lands in the cache.
	//
	// A tile that cannot be written is still a tile that can be drawn, so this
	// does not fail the fetch. It is reported rather than dropped because a full
	// or read-only cache directory otherwise presents as a map that redownloads
	// everything on every launch and never says why.
	if err := os.MkdirAll(filepath.Dir(s.path(l, z, x, y)), 0o755); err != nil {
		diag.Printf("basemap", "caching %s: %v", tileName(l, z, x, y), err)
	} else if err := os.WriteFile(s.path(l, z, x, y), body, 0o644); err != nil {
		diag.Printf("basemap", "caching %s: %v", tileName(l, z, x, y), err)
	}
	s.mu.Lock()
	s.loaded[k] = img
	s.mu.Unlock()
	return nil
}

func decode(b []byte) (*image.RGBA, error) {
	src, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	bounds := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(out, out.Bounds(), src, bounds.Min, draw.Src)
	return out, nil
}

func expand(tmpl string, z, x, y int) string {
	return strings.NewReplacer(
		"{z}", strconv.Itoa(z),
		"{x}", strconv.Itoa(x),
		"{y}", strconv.Itoa(y),
	).Replace(tmpl)
}

// PixelAt samples a layer at a coordinate, without blocking.
//
// Nearest-neighbour out of the tile pyramid, which is exactly what the terrain
// hillshade does with the DEM. One code path, one resampling story, and one
// texture for the whole map rather than one per tile.
func (s *Store) PixelAt(l Layer, lat, lon float64, zoom int) (r, g, b, a uint8, ok bool) {
	if lat > 85.0511 || lat < -85.0511 {
		return 0, 0, 0, 0, false
	}
	n := math.Exp2(float64(zoom))
	px := (lon + 180) / 360 * n * 256
	latRad := lat * math.Pi / 180
	py := (1 - math.Log(math.Tan(latRad)+1/math.Cos(latRad))/math.Pi) / 2 * n * 256

	maxPixel := int(n) * 256
	gx := ((int(px) % maxPixel) + maxPixel) % maxPixel
	gy := int(py)
	if gy < 0 || gy >= maxPixel {
		return 0, 0, 0, 0, false
	}

	img, found := s.Cached(l, zoom, gx/256, gy/256)
	if !found {
		return 0, 0, 0, 0, false
	}
	bounds := img.Bounds()
	ix, iy := gx%256, gy%256
	if ix >= bounds.Dx() || iy >= bounds.Dy() {
		return 0, 0, 0, 0, false
	}
	i := img.PixOffset(bounds.Min.X+ix, bounds.Min.Y+iy)
	return img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3], true
}

// Estimate is the cost of covering a box, before it is paid. ADR-0019 requires
// this before anything is fetched, and the reason is the same here: a pan over
// a county at full zoom is thousands of tiles.
type Estimate struct {
	Tiles      int
	Cached     int
	ToFetch    int
	BytesRough int64
}

func (s *Store) Estimate(l Layer, south, north, west, east float64, zoom int) Estimate {
	tiles := TilesFor(south, north, west, east, zoom)
	e := Estimate{Tiles: len(tiles)}
	for _, t := range tiles {
		if _, ok := s.Cached(l, zoom, t[0], t[1]); ok {
			e.Cached++
		}
	}
	e.ToFetch = e.Tiles - e.Cached
	// Imagery tiles run larger than vector-rendered ones; 40 kB is a middle
	// figure, which is why the field says rough.
	const roughBytesPerTile = 40 << 10
	e.BytesRough = int64(e.ToFetch) * roughBytesPerTile
	return e
}

// Prefetch downloads a box, reporting progress.
//
// Serial on purpose. OpenStreetMap's usage policy asks for no more than two
// concurrent connections, and a workbench that opens twelve is exactly the
// behaviour that gets an address blocked.
func (s *Store) Prefetch(ctx context.Context, l Layer, south, north, west, east float64,
	zoom int, progress func(done, total int)) error {
	tiles := TilesFor(south, north, west, east, zoom)
	for i, t := range tiles {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.Fetch(ctx, l, zoom, t[0], t[1]); err != nil {
			return err
		}
		if progress != nil {
			progress(i+1, len(tiles))
		}
	}
	return nil
}
