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
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg" // Esri serves JPEG; OSM serves PNG.
	_ "image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Kind separates a base layer from something drawn over one.
type Kind string

const (
	// Base layers are opaque and mutually exclusive.
	Base Kind = "base"
	// Overlay layers are mostly transparent and drawn on top — labels, mainly.
	Overlay Kind = "overlay"
)

// Layer is one tiled raster source.
type Layer struct {
	ID   string
	Name string
	Kind Kind
	// Dark reports a layer whose ground is dark, so text drawn over the map
	// can pick an ink that reads on it rather than one chosen for the theme.
	Dark bool

	// URL template with {z}/{x}/{y}.
	URL string

	// MaxZoom is the deepest zoom the source publishes. Asking beyond it
	// returns errors or blank tiles, so requests are clamped and the map
	// upsamples instead — which looks soft rather than broken.
	MaxZoom int

	// Attribution must appear wherever this layer is shown. Not a courtesy:
	// every one of these sources requires it.
	Attribution string

	// Terms is what a human needs to read before this ships.
	Terms string

	// RequiresReview marks a layer whose terms have not been checked against
	// how this application uses it. Such a layer is available to a developer
	// and must not be a default.
	RequiresReview bool
}

// Layers is what this build knows about.
//
// Deliberately few, and every one a source that publishes a documented tile
// service. Scraping a consumer map product is both a licensing problem and a
// technical one — the URLs change without notice — so nothing here does it.
func Layers() []Layer {
	return []Layer{
		{
			// CARTO's basemaps are built to be a background for data, which is
			// exactly this use, and they are quiet enough that a coverage
			// raster over them stays readable. OSM's standard style is not —
			// it is a map in its own right and fights anything drawn on it.
			ID: "carto-light", Name: "Light", Kind: Base, MaxZoom: 20,
			URL:         "https://basemaps.cartocdn.com/light_all/{z}/{x}/{y}.png",
			Attribution: "(c) OpenStreetMap contributors, (c) CARTO",
			Terms: "CARTO basemaps. Free for use with attribution; a token is " +
				"required above their fair-use volume, which a workbench prefetching " +
				"whole counties could reach.",
			RequiresReview: true,
		},
		{
			ID: "carto-dark", Name: "Dark", Kind: Base, Dark: true, MaxZoom: 20,
			URL:            "https://basemaps.cartocdn.com/dark_all/{z}/{x}/{y}.png",
			Attribution:    "(c) OpenStreetMap contributors, (c) CARTO",
			Terms:          "As CARTO light above.",
			RequiresReview: true,
		},
		{
			ID: "osm", Name: "Roads", Kind: Base, MaxZoom: 19,
			URL:         "https://tile.openstreetmap.org/{z}/{x}/{y}.png",
			Attribution: "(c) OpenStreetMap contributors",
			Terms: "OpenStreetMap Foundation tile usage policy. Requires a valid " +
				"identifying User-Agent, forbids bulk downloading, and expects heavy " +
				"users to run their own tile server. A workbench prefetching a county " +
				"is plausibly bulk downloading.",
			RequiresReview: true,
		},
		{
			ID: "esri-imagery", Name: "Satellite", Kind: Base, Dark: true, MaxZoom: 19,
			URL: "https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/" +
				"MapServer/tile/{z}/{y}/{x}",
			Attribution: "Imagery (c) Esri, Maxar, Earthstar Geographics",
			Terms: "Esri ArcGIS Online basemap. Free tiers exist for some uses and " +
				"not others; caching to disk in particular is restricted under some " +
				"Esri terms.",
			RequiresReview: true,
		},
		{
			ID: "esri-topo", Name: "Topographic", Kind: Base, MaxZoom: 19,
			URL: "https://server.arcgisonline.com/ArcGIS/rest/services/World_Topo_Map/" +
				"MapServer/tile/{z}/{y}/{x}",
			Attribution:    "(c) Esri, contributors",
			Terms:          "As Esri imagery above.",
			RequiresReview: true,
		},
		{
			// CARTO publishes its labels as their own transparent layer,
			// which is what lets place names ride ABOVE a coverage raster
			// instead of drowning under it.
			ID: "carto-dark-labels", Name: "Labels (dark)", Kind: Overlay, Dark: true, MaxZoom: 20,
			URL:            "https://basemaps.cartocdn.com/dark_only_labels/{z}/{x}/{y}.png",
			Attribution:    "(c) OpenStreetMap contributors, (c) CARTO",
			Terms:          "As CARTO light above.",
			RequiresReview: true,
		},
		{
			ID: "carto-light-labels", Name: "Labels (light)", Kind: Overlay, MaxZoom: 20,
			URL:            "https://basemaps.cartocdn.com/light_only_labels/{z}/{x}/{y}.png",
			Attribution:    "(c) OpenStreetMap contributors, (c) CARTO",
			Terms:          "As CARTO light above.",
			RequiresReview: true,
		},
		{
			// MaxZoom is what the service actually draws, not what it
			// answers: past 17 it returns 200s full of transparent
			// nothing, which cached as roads that vanish when zoomed in.
			ID: "esri-roads", Name: "Roads (overlay)", Kind: Overlay, MaxZoom: 17,
			URL: "https://server.arcgisonline.com/ArcGIS/rest/services/Reference/" +
				"World_Transportation/MapServer/tile/{z}/{y}/{x}",
			Attribution:    "(c) Esri, contributors",
			Terms:          "As Esri imagery above.",
			RequiresReview: true,
		},
		{
			ID: "esri-labels", Name: "Places and roads", Kind: Overlay, MaxZoom: 17,
			URL: "https://server.arcgisonline.com/ArcGIS/rest/services/Reference/" +
				"World_Boundaries_and_Places/MapServer/tile/{z}/{y}/{x}",
			Attribution:    "(c) Esri, contributors",
			Terms:          "As Esri imagery above.",
			RequiresReview: true,
		},
	}
}

// OverlaysFor is what belongs above a coverage raster for a given base:
// roads first, then labels, so the ground's structure and its names both
// come through the picture instead of drowning under it. Bases with
// everything baked in and no separate layers answer empty.
func OverlaysFor(baseID string) []Layer {
	// The streets come through the raster as a ghost of the base itself -
	// every road at every zoom, no second source, no doubled names - so
	// the overlay stack is labels only, from the base's own family. The
	// exceptions carry Esri layers because their base has nothing baked
	// to ghost (imagery) or no separate labels of its own.
	ids := map[string][]string{
		"carto-dark":   {"carto-dark-labels"},
		"carto-light":  {"carto-light-labels"},
		"esri-imagery": {"esri-roads", "esri-labels"},
		"esri-topo":    {"esri-labels"},
	}[baseID]
	var out []Layer
	for _, id := range ids {
		if l, ok := ByID(id); ok {
			out = append(out, l)
		}
	}
	return out
}

// ByID finds a layer.
func ByID(id string) (Layer, bool) {
	for _, l := range Layers() {
		if l.ID == id {
			return l, true
		}
	}
	return Layer{}, false
}

// ZoomFor picks the tile zoom whose pixels are nearest the view's own scale.
//
// Capped at the layer's maximum, and never below 1. ADR-0019's finding applies
// unchanged: naive zoom selection is what turns a map pan into a 700-tile
// download, and choosing the zoom that matches the screen is what stops it.
func ZoomFor(metresPerPixel, latDeg float64, l Layer) int {
	if metresPerPixel <= 0 {
		return 12
	}
	// Web Mercator ground resolution at zoom z.
	const equator = 156543.033928
	res := equator * math.Cos(latDeg*math.Pi/180)
	z := int(math.Round(math.Log2(res / metresPerPixel)))
	maxZoom := l.MaxZoom
	if maxZoom <= 0 {
		maxZoom = 18
	}
	if z > maxZoom {
		z = maxZoom
	}
	if z < 1 {
		z = 1
	}
	return z
}

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
		return fmt.Errorf("basemap: fetch %s: %w", url, err)
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
		return fmt.Errorf("basemap: %s returned %s", url, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	img, err := decode(body)
	if err != nil {
		return fmt.Errorf("basemap: %s: %w", url, err)
	}

	// Decoded before it is written, so an error page never lands in the cache.
	if err := os.MkdirAll(filepath.Dir(s.path(l, z, x, y)), 0o755); err == nil {
		_ = os.WriteFile(s.path(l, z, x, y), body, 0o644)
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
