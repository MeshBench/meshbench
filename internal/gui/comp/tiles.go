package comp

import (
	"context"
	"image"
	"math"
	"sync"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/MeshBench/meshbench/internal/basemap"
)

// Tiles draws the basemap under everything else.
//
// It reuses internal/basemap, which already has the disk cache, the Web
// Mercator arithmetic and the "never block a redraw on the network" rule that
// took work to get right the first time. Drawing only ever reads cached tiles;
// anything missing is fetched on a worker and appears on a later frame, so a
// slow network makes the map sparse rather than making the window stop
// painting.
type Tiles struct {
	Store *basemap.Store
	Layer basemap.Layer

	mu      sync.Mutex
	fetched map[string]bool
	ops     map[string]uploaded
	// decoding is the set of tiles being read off disk on a worker. Decoding
	// a PNG is tens of microseconds; a screenful of new ones is tens of
	// milliseconds, and doing that inside a frame is the stutter somebody
	// feels the first time they pan somewhere new.
	decoding map[string]bool
	// ready carries decoded tiles back to the frame thread, which is the only
	// place a paint.ImageOp may be made.
	ready map[string]*image.RGBA
}

// uploaded is a tile that has been decoded once and handed to the GPU.
type uploaded struct {
	op   paint.ImageOp
	size image.Point
}

// NewTiles prepares a tile layer. A nil store is legal and draws nothing,
// which is what an offline first run looks like.
func NewTiles(cacheDir, layerID string) *Tiles {
	t := &Tiles{fetched: map[string]bool{}, ops: map[string]uploaded{},
		decoding: map[string]bool{}, ready: map[string]*image.RGBA{}}
	if l, ok := basemap.ByID(layerID); ok {
		t.Layer = l
	}
	if st, err := basemap.NewStore(cacheDir); err == nil {
		t.Store = st
	}
	return t
}

// SetLayer switches the basemap by its ID.
//
// Everything cached - uploads, decodes, fetch marks - is already keyed by
// layer, so nothing needs clearing and switching back to a map is instant.
func (t *Tiles) SetLayer(id string) {
	if l, ok := basemap.ByID(id); ok {
		t.Layer = l
	}
}

// Draw paints the tiles covering a viewport, and returns how many were
// actually available. The caller decides what to say about the difference.
func (t *Tiles) Draw(gtx layout.Context, sz image.Point, centreLat, centreLon, zoomPxPerDeg float64) (drawn, want int) {
	if t.Store == nil || t.Layer.ID == "" {
		return 0, 0
	}
	// Metres per pixel at this latitude, from the pixels-per-degree the map is
	// using, so the tile zoom matches what is on screen rather than a guess.
	metresPerDeg := 111320 * math.Cos(centreLat*math.Pi/180)
	if metresPerDeg < 1 {
		metresPerDeg = 1
	}
	mpp := metresPerDeg / zoomPxPerDeg
	z := basemap.ZoomFor(mpp, centreLat, t.Layer)

	halfLat := float64(sz.Y) / 2 / zoomPxPerDeg
	cos := math.Cos(centreLat * math.Pi / 180)
	if cos < 0.01 {
		cos = 0.01
	}
	halfLon := float64(sz.X) / 2 / (zoomPxPerDeg * cos)
	south, north := centreLat-halfLat, centreLat+halfLat
	west, east := centreLon-halfLon, centreLon+halfLon

	for _, xy := range basemap.TilesFor(south, north, west, east, z) {
		want++
		x, y := xy[0], xy[1]

		// The upload cache is asked first, and the store only on a miss.
		// Store.Cached reads and decodes the PNG from disk, so asking it once
		// per tile per frame decoded the whole visible basemap sixty times a
		// second - 12% of the map's CPU profile, to produce an image that had
		// not changed since it was downloaded.
		iop, size, ok := t.cachedOp(z, x, y)
		if !ok {
			// Anything a worker finished since the last frame, uploaded here
			// where it is legal to do so.
			if img, done := t.takeDecoded(z, x, y); done {
				iop, size = t.putOp(z, x, y, img)
			} else {
				t.decodeOnce(z, x, y)
				continue
			}
		}

		// Where this tile's north-west corner lands on screen.
		lat, lon := tileNW(x, y, z)
		px := float64(sz.X)/2 + (lon-centreLon)*cos*zoomPxPerDeg
		py := float64(sz.Y)/2 - (lat-centreLat)*zoomPxPerDeg
		// And its south-east corner, which gives the on-screen size without
		// assuming tiles are square in degrees. They are not, away from the
		// equator.
		lat2, lon2 := tileNW(x+1, y+1, z)
		w := (lon2 - lon) * cos * zoomPxPerDeg
		h := (lat - lat2) * zoomPxPerDeg
		if w < 1 || h < 1 {
			continue
		}

		off := op.Offset(image.Pt(int(px), int(py))).Push(gtx.Ops)
		cl := clip.Rect{Max: image.Pt(int(w)+1, int(h)+1)}.Push(gtx.Ops)
		// Scale the 256 pixel tile to the space it covers.
		sc := op.Affine(f32Scale(float32(w)/float32(size.X),
			float32(h)/float32(size.Y))).Push(gtx.Ops)
		iop.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		sc.Pop()
		cl.Pop()
		off.Pop()
		drawn++
	}
	return drawn, want
}

// cachedOp returns an already-uploaded tile, and whether there was one.
func (t *Tiles) cachedOp(z, x, y int) (paint.ImageOp, image.Point, bool) {
	k := tileKey(t.Layer.ID, z, x, y)
	t.mu.Lock()
	defer t.mu.Unlock()
	c, ok := t.ops[k]
	return c.op, c.size, ok
}

// putOp uploads a tile once and remembers it. This is the difference between
// a map that costs nothing to redraw and one that costs its whole area every
// frame.
func (t *Tiles) putOp(z, x, y int, img *image.RGBA) (paint.ImageOp, image.Point) {
	k := tileKey(t.Layer.ID, z, x, y)
	c := uploaded{op: paint.NewImageOp(img), size: img.Bounds().Size()}
	t.mu.Lock()
	t.ops[k] = c
	t.mu.Unlock()
	return c.op, c.size
}

// fetchOnce asks for a missing tile exactly once per session.
func (t *Tiles) fetchOnce(z, x, y int) {
	k := tileKey(t.Layer.ID, z, x, y)
	t.mu.Lock()
	if t.fetched[k] {
		t.mu.Unlock()
		return
	}
	t.fetched[k] = true
	t.mu.Unlock()
	go func() { _ = t.Store.Fetch(context.Background(), t.Layer, z, x, y) }()
}

// decodeOnce reads and decodes a tile on a worker, or starts the download if
// it is not on disk yet. Either way this frame draws without it.
func (t *Tiles) decodeOnce(z, x, y int) {
	k := tileKey(t.Layer.ID, z, x, y)
	t.mu.Lock()
	if t.decoding[k] {
		t.mu.Unlock()
		return
	}
	t.decoding[k] = true
	t.mu.Unlock()

	go func() {
		img, have := t.Store.Cached(t.Layer, z, x, y)
		t.mu.Lock()
		delete(t.decoding, k)
		if have {
			t.ready[k] = img
		}
		t.mu.Unlock()
		if !have {
			t.fetchOnce(z, x, y)
		}
	}()
}

// takeDecoded hands over a tile a worker finished, once.
func (t *Tiles) takeDecoded(z, x, y int) (*image.RGBA, bool) {
	k := tileKey(t.Layer.ID, z, x, y)
	t.mu.Lock()
	defer t.mu.Unlock()
	img, ok := t.ready[k]
	if ok {
		delete(t.ready, k)
	}
	return img, ok
}

func tileKey(id string, z, x, y int) string {
	return id + "/" + itoa(z) + "/" + itoa(x) + "/" + itoa(y)
}

// tileNW is the north-west corner of a Web Mercator tile.
func tileNW(x, y, zoom int) (lat, lon float64) {
	n := math.Pow(2, float64(zoom))
	lon = float64(x)/n*360 - 180
	latRad := math.Atan(math.Sinh(math.Pi * (1 - 2*float64(y)/n)))
	return latRad * 180 / math.Pi, lon
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [12]byte
	p := len(b)
	for v > 0 {
		p--
		b[p] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		p--
		b[p] = '-'
	}
	return string(b[p:])
}
