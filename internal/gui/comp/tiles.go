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

	"github.com/A13xB0/meshcoresim/internal/basemap"
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
	ops     map[string]paint.ImageOp
}

// NewTiles prepares a tile layer. A nil store is legal and draws nothing,
// which is what an offline first run looks like.
func NewTiles(cacheDir, layerID string) *Tiles {
	t := &Tiles{fetched: map[string]bool{}, ops: map[string]paint.ImageOp{}}
	if l, ok := basemap.ByID(layerID); ok {
		t.Layer = l
	}
	if st, err := basemap.NewStore(cacheDir); err == nil {
		t.Store = st
	}
	return t
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
		img, ok := t.Store.Cached(t.Layer, z, x, y)
		if !ok {
			t.fetchOnce(z, x, y)
			continue
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
		iop := t.imageOp(z, x, y, img)
		// Scale the 256 pixel tile to the space it covers.
		sc := op.Affine(f32Scale(float32(w)/float32(img.Bounds().Dx()),
			float32(h)/float32(img.Bounds().Dy()))).Push(gtx.Ops)
		iop.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		sc.Pop()
		cl.Pop()
		off.Pop()
		drawn++
	}
	return drawn, want
}

// imageOp caches the upload, so a tile is not re-uploaded to the GPU on every
// frame. This is the difference between a map that costs nothing to redraw and
// one that costs its whole area every frame.
func (t *Tiles) imageOp(z, x, y int, img *image.RGBA) paint.ImageOp {
	k := tileKey(t.Layer.ID, z, x, y)
	t.mu.Lock()
	defer t.mu.Unlock()
	if op, ok := t.ops[k]; ok {
		return op
	}
	op := paint.NewImageOp(img)
	t.ops[k] = op
	return op
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
