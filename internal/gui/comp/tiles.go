package comp

import (
	"context"
	"image"
	"math"
	"sync"
	"time"

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
	// retryAfter is when a tile whose download failed may be asked for again.
	// Without it a failure is permanent: fetched is set before the request and
	// used as the "already asked" mark, so a tile that failed once would never
	// be requested again for the rest of the session and stayed blank.
	retryAfter map[string]time.Time
	ops        map[string]uploaded
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
		decoding: map[string]bool{}, ready: map[string]*image.RGBA{},
		retryAfter: map[string]time.Time{}}
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
	z := basemap.ZoomFor(metresPerPixel(zoomPxPerDeg), centreLat, t.Layer)

	cos := math.Cos(centreLat * math.Pi / 180)
	if cos < 0.01 {
		cos = 0.01
	}
	south, north, west, east := viewportBounds(sz, centreLat, centreLon, zoomPxPerDeg)

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

// metresPerPixel is the map's scale, which does not depend on latitude.
//
// It looks as though it should. The projection scales longitude by the cosine
// of the centre latitude, so a degree of longitude is fewer pixels up north -
// but a degree of longitude is fewer metres up north by exactly the same
// factor, and the two cancel. The map is locally isotropic: 111320/Zoom metres
// to a pixel, across and down, everywhere.
//
// This used to fold the cosine in once, which made the scale look finer than
// it was and sent ZoomFor log2(1/cos) levels too deep - one extra level in
// Scotland, three at latitude 84. Every one of those is four times the tiles,
// fetched and decoded and drawn, for a picture no sharper than the screen can
// show.
func metresPerPixel(zoomPxPerDeg float64) float64 {
	if zoomPxPerDeg <= 0 {
		return math.Inf(1)
	}
	// Metres in a degree of latitude, which is the axis the zoom is in.
	const metresPerDegLat = 111320
	return metresPerDegLat / zoomPxPerDeg
}

// viewportBounds is the box on the ground the viewport covers.
//
// Zoomed out, this is much larger than the planet - hundreds of degrees of
// latitude at the minimum zoom - and that is not a mistake to correct here.
// It is what the viewport covers, and clamping it to the world is the tile
// grid's business, where the world is what the grid is made of. Correcting it
// here as well would mean two places that both half-know the rule.
func viewportBounds(sz image.Point, centreLat, centreLon, zoomPxPerDeg float64) (south, north, west, east float64) {
	if zoomPxPerDeg <= 0 {
		return centreLat, centreLat, centreLon, centreLon
	}
	cos := math.Cos(centreLat * math.Pi / 180)
	if cos < 0.01 {
		cos = 0.01
	}
	halfLat := float64(sz.Y) / 2 / zoomPxPerDeg
	halfLon := float64(sz.X) / 2 / (zoomPxPerDeg * cos)
	return centreLat - halfLat, centreLat + halfLat,
		centreLon - halfLon, centreLon + halfLon
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

// tileRetryDelay is how long a tile whose download failed waits before it may
// be asked for again. Long enough that a burst of failures - which is what a
// zoom into uncached country produces, a hundred requests at once against a
// server with its own opinion about that - does not turn into a retry storm,
// short enough that the map fills in while somebody is still looking at it.
const tileRetryDelay = 3 * time.Second

// fetchOnce asks for a missing tile, once at a time and not forever.
//
// The mark goes on before the request, so a tile is not requested twice
// concurrently. It used to stay on regardless of how the request went, with
// the error discarded - so a single failure, and a burst of them is the normal
// shape of a zoom, left that tile blank for the rest of the session with
// nothing retrying it. A failure now clears the mark and holds the tile back
// for tileRetryDelay instead.
func (t *Tiles) fetchOnce(z, x, y int) {
	k := tileKey(t.Layer.ID, z, x, y)
	t.mu.Lock()
	if !t.mayFetch(k, time.Now()) {
		t.mu.Unlock()
		return
	}
	t.fetched[k] = true
	t.mu.Unlock()
	go func() {
		if err := t.Store.Fetch(context.Background(), t.Layer, z, x, y); err != nil {
			t.mu.Lock()
			delete(t.fetched, k)
			t.retryAfter[k] = time.Now().Add(tileRetryDelay)
			t.mu.Unlock()
		}
	}()
}

// mayFetch reports whether this tile may be asked for now: not already in
// flight or done, and not inside the wait a previous failure earned it.
// Caller holds mu.
func (t *Tiles) mayFetch(k string, now time.Time) bool {
	if t.fetched[k] {
		return false
	}
	if until, waiting := t.retryAfter[k]; waiting && now.Before(until) {
		return false
	}
	return true
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
