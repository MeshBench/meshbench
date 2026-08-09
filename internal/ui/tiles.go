package ui

import (
	"context"
	"image"
	"math"
	"sync"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/basemap"
)

// tileCache draws a map the way a map is drawn: one textured quad per tile,
// sampled by the GPU.
//
// The previous approach resampled every visible tile into one CPU image on
// every view change — 150,000 samples per pan, nearest-neighbour, one texture
// upload — which was slow, soft, and left holes wherever a tile had not been
// reached yet. It was a tile renderer written badly. This is the same thing
// every map library does instead: upload each tile once, then let the hardware
// scale and filter it.
//
// The consequences are not subtle. A pan costs nothing but a few draw calls,
// tiles arrive one at a time instead of the whole view redrawing, and the
// imagery is at its own resolution rather than at whatever the screen sampled.
type tileCache struct {
	mu    sync.Mutex
	tex   map[string]*imgui.TextureRef
	want  map[string]tileReq
	ready chan tileResult

	// inflight bounds concurrent fetches. Two is what OpenStreetMap's usage
	// policy asks for, and a workbench that opens twelve is how an address gets
	// blocked.
	inflight int
}

type tileReq struct {
	layer   basemap.Layer
	z, x, y int
}

type tileResult struct {
	key string
	img *image.RGBA
}

func newTileCache() *tileCache {
	return &tileCache{
		tex:   map[string]*imgui.TextureRef{},
		want:  map[string]tileReq{},
		ready: make(chan tileResult, 32),
	}
}

func tileKey(l basemap.Layer, z, x, y int) string {
	return l.ID + ":" + itoa(z) + "/" + itoa(x) + "/" + itoa(y)
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// upload takes finished tiles and turns them into textures.
//
// On the frame thread, because creating a GPU texture from another goroutine is
// not safe. A handful per frame so a view over new ground fills in smoothly
// rather than stalling on the frame that happens to catch fifty of them.
func (c *tileCache) upload(b textureMaker) {
	for i := 0; i < 8; i++ {
		select {
		case r := <-c.ready:
			c.mu.Lock()
			c.inflight--
			if r.img != nil {
				tex := b.CreateTextureRgba(r.img, r.img.Bounds().Dx(), r.img.Bounds().Dy())
				c.tex[r.key] = &tex
			} else {
				// Remembered as absent. Without this a tile the source does not
				// have is requested again on every single frame.
				c.tex[r.key] = nil
			}
			delete(c.want, r.key)
			c.mu.Unlock()
		default:
			return
		}
	}
}

type textureMaker interface {
	CreateTextureRgba(img *image.RGBA, w, h int) imgui.TextureRef
}

// get returns a tile's texture if it is ready, and starts fetching it if not.
func (c *tileCache) get(store *basemap.Store, l basemap.Layer, z, x, y int, allowFetch bool) *imgui.TextureRef {
	key := tileKey(l, z, x, y)

	c.mu.Lock()
	if tex, known := c.tex[key]; known {
		c.mu.Unlock()
		return tex
	}
	_, queued := c.want[key]
	busy := c.inflight >= 2
	if queued || (busy && allowFetch) {
		c.mu.Unlock()
		return nil
	}
	c.want[key] = tileReq{l, z, x, y}
	c.inflight++
	c.mu.Unlock()

	go func() {
		// Cache first, always. Only reach for the network if the operator has
		// allowed it.
		if img, ok := store.Cached(l, z, x, y); ok {
			c.ready <- tileResult{key, img}
			return
		}
		if !allowFetch {
			c.ready <- tileResult{key, nil}
			return
		}
		if err := store.Fetch(context.Background(), l, z, x, y); err != nil {
			c.ready <- tileResult{key, nil}
			return
		}
		img, _ := store.Cached(l, z, x, y)
		c.ready <- tileResult{key, img}
	}()
	return nil
}

// forget drops every texture, for when the layer changes.
func (c *tileCache) forget() {
	c.mu.Lock()
	c.tex = map[string]*imgui.TextureRef{}
	c.mu.Unlock()
}

// drawTiles paints one layer across the view.
//
// Each tile is placed at its true geographic corners, so the projection is the
// only thing deciding where imagery lands and it is the same projection the
// nodes are drawn with. Scaling a whole-view image, as before, put the two
// subtly out of step at every zoom that was not a power of two.
func (a *App) drawTiles(origin imgui.Vec2, w, h float32, l basemap.Layer, allowFetch bool) int {
	if a.bmStore == nil {
		return 0
	}
	zoom := basemap.ZoomFor(a.view.MetresPerPixel, a.view.CentreLat, l)
	south, north, west, east := a.view.Bounds()

	dl := imgui.WindowDrawList()
	drawn := 0
	for _, xy := range basemap.TilesFor(south, north, west, east, zoom) {
		tex := a.tiles.get(a.bmStore, l, zoom, xy[0], xy[1], allowFetch)
		if tex == nil {
			continue
		}
		// A tile's own corners, projected. Web Mercator y is not linear in
		// latitude, so a tile is not a fixed number of pixels tall and using
		// one is how imagery drifts away from the nodes drawn on it.
		nwLat, nwLon := tileNW(xy[0], xy[1], zoom)
		seLat, seLon := tileNW(xy[0]+1, xy[1]+1, zoom)

		x0, y0 := a.view.LatLonToScreen(nwLat, nwLon)
		x1, y1 := a.view.LatLonToScreen(seLat, seLon)
		dl.AddImage(*tex,
			imgui.NewVec2(origin.X+float32(x0), origin.Y+float32(y0)),
			imgui.NewVec2(origin.X+float32(x1), origin.Y+float32(y1)))
		drawn++
	}
	return drawn
}

// tileNW is the north-west corner of a slippy-map tile.
func tileNW(x, y, zoom int) (lat, lon float64) {
	n := math.Exp2(float64(zoom))
	lon = float64(x)/n*360 - 180
	latRad := math.Atan(math.Sinh(math.Pi * (1 - 2*float64(y)/n)))
	return latRad * 180 / math.Pi, lon
}
