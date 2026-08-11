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

	inflight int
}

// maxInflight bounds concurrent tile fetches.
//
// Six is a compromise. OpenStreetMap's usage policy asks for two, which makes
// a pan crawl; CARTO and Esri are CDNs that expect browser-like concurrency,
// and a browser opens six. The layer that needs two is the one whose terms say
// so, and honouring that per layer is work for when the terms are settled.
const maxInflight = 6

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

// peek returns a texture only if it is already uploaded, without queuing work.
func (c *tileCache) peek(l basemap.Layer, z, x, y int) *imgui.TextureRef {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tex[tileKey(l, z, x, y)]
}

// get returns a tile's texture if it is ready, and starts fetching it if not.
func (c *tileCache) get(store *basemap.Store, l basemap.Layer, z, x, y int) *imgui.TextureRef {
	key := tileKey(l, z, x, y)

	c.mu.Lock()
	if tex, known := c.tex[key]; known {
		c.mu.Unlock()
		return tex
	}
	_, queued := c.want[key]
	if queued || c.inflight >= maxInflight {
		c.mu.Unlock()
		return nil
	}
	c.want[key] = tileReq{l, z, x, y}
	c.inflight++
	c.mu.Unlock()

	go func() {
		// Cache first, always.
		if img, ok := store.Cached(l, z, x, y); ok {
			c.ready <- tileResult{key, img}
			return
		}
		// Then the network, unless the operator asked for offline. A map that
		// needs a checkbox ticked before it will load is not a map, and every
		// other one on the machine just fetches what is on screen.
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
// nodes are drawn with.
//
// A tile that is not ready yet falls back to its parent, scaled. That is what
// stops the map appearing a square at a time: the coarser tile is already in
// memory from the zoom level above, so there is always something to draw and
// detail sharpens rather than materialising out of nothing.
func (a *App) drawTiles(origin imgui.Vec2, w, h float32, l basemap.Layer) int {
	if a.bmStore == nil {
		return 0
	}
	zoom := basemap.ZoomFor(a.view.MetresPerPixel, a.view.CentreLat, l)
	south, north, west, east := a.view.Bounds()

	dl := imgui.WindowDrawList()
	// Clip to the map. Without this the draw list paints tiles over the panels
	// below and beside it, which is what "the map bleeds through" was.
	dl.PushClipRectV(origin, imgui.NewVec2(origin.X+w, origin.Y+h), true)
	defer dl.PopClipRect()

	drawn := 0
	for _, xy := range basemap.TilesFor(south, north, west, east, zoom) {
		nwLat, nwLon := tileNW(xy[0], xy[1], zoom)
		seLat, seLon := tileNW(xy[0]+1, xy[1]+1, zoom)
		x0, y0 := a.view.LatLonToScreen(nwLat, nwLon)
		x1, y1 := a.view.LatLonToScreen(seLat, seLon)
		pMin := imgui.NewVec2(origin.X+float32(x0), origin.Y+float32(y0))
		pMax := imgui.NewVec2(origin.X+float32(x1), origin.Y+float32(y1))

		if tex := a.tiles.get(a.bmStore, l, zoom, xy[0], xy[1]); tex != nil {
			dl.AddImage(*tex, pMin, pMax)
			drawn++
			continue
		}
		if tex, u0, v0, u1, v1, ok := a.parentTile(l, zoom, xy[0], xy[1]); ok {
			dl.AddImageV(*tex, pMin, pMax,
				imgui.NewVec2(u0, v0), imgui.NewVec2(u1, v1), 0xFFFFFFFF)
			drawn++
		}
	}
	return drawn
}

// parentTile finds the nearest coarser tile covering this one, and the part of
// it to sample.
//
// Up to four levels, which is a sixteenth of the detail and still better than a
// hole. Beyond that the blur is worse than an honest gap.
func (a *App) parentTile(l basemap.Layer, z, x, y int) (*imgui.TextureRef, float32, float32, float32, float32, bool) {
	for up := 1; up <= 4 && z-up >= 0; up++ {
		px, py := x>>up, y>>up
		tex := a.tiles.peek(l, z-up, px, py)
		if tex == nil {
			continue
		}
		span := 1 << up
		u0 := float32(x-(px<<up)) / float32(span)
		v0 := float32(y-(py<<up)) / float32(span)
		return tex, u0, v0, u0 + 1/float32(span), v0 + 1/float32(span), true
	}
	return nil, 0, 0, 0, 0, false
}

// tileNW is the north-west corner of a slippy-map tile.
func tileNW(x, y, zoom int) (lat, lon float64) {
	n := math.Exp2(float64(zoom))
	lon = float64(x)/n*360 - 180
	latRad := math.Atan(math.Sinh(math.Pi * (1 - 2*float64(y)/n)))
	return latRad * 180 / math.Pi, lon
}
