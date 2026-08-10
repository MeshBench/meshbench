package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/A13xB0/meshcoresim/internal/basemap"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// MapView is the geographic viewport: what part of the world is on screen.
//
// A workbench is arranged around a map, not around a list. A list of four nodes
// is fine and a list of four hundred is unusable, and the questions people
// actually ask — where is the gap, which repeater covers this valley, where
// should the next one go — are spatial questions that a list cannot answer at
// any length.
type MapView struct {
	CentreLat float64
	CentreLon float64

	// MetresPerPixel is the scale. Stored rather than a zoom level because
	// every distance in the engine is in metres and converting at each use is
	// where sign and factor errors live.
	MetresPerPixel float64

	// Width and Height in pixels, set from the panel each frame.
	Width, Height int
}

// LatLonToScreen projects a position to a pixel offset from the panel's origin.
func (m MapView) LatLonToScreen(lat, lon float64) (x, y float64) {
	// Local equirectangular about the view centre. Correct to well under a
	// pixel over the tens of kilometres a mesh spans, and it keeps the inverse
	// exact — which matters because clicking is the inverse and a projection
	// that does not round-trip puts a node where nobody clicked.
	const mPerDegLat = 111_320.0
	mPerDegLon := mPerDegLat * math.Cos(m.CentreLat*math.Pi/180)

	dx := (lon - m.CentreLon) * mPerDegLon / m.MetresPerPixel
	dy := (lat - m.CentreLat) * mPerDegLat / m.MetresPerPixel
	return float64(m.Width)/2 + dx, float64(m.Height)/2 - dy
}

// ScreenToLatLon is the exact inverse of LatLonToScreen.
func (m MapView) ScreenToLatLon(x, y float64) (lat, lon float64) {
	const mPerDegLat = 111_320.0
	mPerDegLon := mPerDegLat * math.Cos(m.CentreLat*math.Pi/180)

	dx := x - float64(m.Width)/2
	dy := float64(m.Height)/2 - y
	return m.CentreLat + dy*m.MetresPerPixel/mPerDegLat,
		m.CentreLon + dx*m.MetresPerPixel/mPerDegLon
}

// Bounds is the geographic extent currently on screen.
func (m MapView) Bounds() (south, north, west, east float64) {
	north, west = m.ScreenToLatLon(0, 0)
	south, east = m.ScreenToLatLon(float64(m.Width), float64(m.Height))
	return south, north, west, east
}

// FitTo frames a set of nodes with a margin.
func (m *MapView) FitTo(nodes []scenario.Node, w, h int) {
	if len(nodes) == 0 || w <= 0 || h <= 0 {
		return
	}
	south, north := math.Inf(1), math.Inf(-1)
	west, east := math.Inf(1), math.Inf(-1)
	for _, n := range nodes {
		south = math.Min(south, n.Position.Lat)
		north = math.Max(north, n.Position.Lat)
		west = math.Min(west, n.Position.Lon)
		east = math.Max(east, n.Position.Lon)
	}
	m.CentreLat, m.CentreLon = (south+north)/2, (west+east)/2
	m.Width, m.Height = w, h

	const mPerDegLat = 111_320.0
	mPerDegLon := mPerDegLat * math.Cos(m.CentreLat*math.Pi/180)
	spanX := math.Max((east-west)*mPerDegLon, 2000)
	spanY := math.Max((north-south)*mPerDegLat, 2000)

	// 1.4 so the outermost nodes are not against the edge, where a marker is
	// half off screen and easy to miss entirely.
	m.MetresPerPixel = 1.4 * math.Max(spanX/float64(w), spanY/float64(h))
}

// ZoomAt scales the view about a screen point, so the ground under the cursor
// stays under the cursor. Zooming about the centre instead makes the map slide
// away from whatever the user was looking at.
func (m *MapView) ZoomAt(x, y, factor float64) {
	anchorLat, anchorLon := m.ScreenToLatLon(x, y)
	m.MetresPerPixel = clampF(m.MetresPerPixel/factor, 0.5, 5000)

	// Latitude first, then longitude, and the order is not stylistic. Metres
	// per degree of longitude depends on the centre latitude, so correcting
	// latitude invalidates any longitude correction computed alongside it. Done
	// together, the anchor drifts east or west a little on every zoom — small
	// enough to read as imprecision and cumulative enough to lose the point you
	// were looking at.
	newLat, _ := m.ScreenToLatLon(x, y)
	m.CentreLat += anchorLat - newLat

	_, newLon := m.ScreenToLatLon(x, y)
	m.CentreLon += anchorLon - newLon
}

// PanPixels moves the view by a screen delta.
func (m *MapView) PanPixels(dx, dy float64) {
	const mPerDegLat = 111_320.0
	// Latitude first, for the same reason as ZoomAt: the longitude scale
	// depends on where the centre latitude ends up, not where it started.
	m.CentreLat += dy * m.MetresPerPixel / mPerDegLat
	mPerDegLon := mPerDegLat * math.Cos(m.CentreLat*math.Pi/180)
	m.CentreLon -= dx * m.MetresPerPixel / mPerDegLon
}

// NodeAt finds the node whose marker is under a screen point, or -1.
//
// Nearest within a radius rather than first match: markers overlap at low zoom,
// and picking the first in list order means the node a user can see on top is
// not the one that gets selected.
func (m MapView) NodeAt(nodes []scenario.Node, x, y, radiusPx float64) int {
	best, bestDist := -1, radiusPx*radiusPx
	for i, n := range nodes {
		nx, ny := m.LatLonToScreen(n.Position.Lat, n.Position.Lon)
		d := (nx-x)*(nx-x) + (ny-y)*(ny-y)
		if d <= bestDist {
			best, bestDist = i, d
		}
	}
	return best
}

// terrainImage renders the visible extent as a shaded relief image.
//
// Hillshade rather than a colour ramp: a ramp tells you altitude, and what
// matters for radio is *shape* — where the ridges and the valleys are. A shaded
// image makes an obstruction visible as a landform rather than as a colour that
// has to be decoded against a legend.
//
// It samples through CachedTerrain where the source offers one, so a redraw
// never waits on a download. Where a tile is not yet in memory it draws a gap,
// which is honest and instant; the alternative is a window that stops painting
// and looks like a crash.
// Basemap is a source of imagery under the simulation. Optional: the hillshade
// alone is a complete map, and a build with no network has to stay usable.
type Basemap interface {
	PixelAt(l basemap.Layer, lat, lon float64, zoom int) (r, g, b, a uint8, ok bool)
}

// Composite is what to draw under the nodes.
type Composite struct {
	// Base is the imagery layer, if any. Empty ID means hillshade only.
	Base    basemap.Layer
	HasBase bool

	// Labels is an overlay drawn on top of everything.
	Labels    basemap.Layer
	HasLabels bool

	// ShadeMix blends the DEM hillshade over the imagery, in [0,1]. Shading
	// over satellite is not decoration: imagery flattens terrain badly in flat
	// light, and a ridge that decides a link can be invisible in it.
	ShadeMix float64
}

func terrainImage(t Terrain, v MapView, step int) *image.RGBA {
	sample := t.ElevationM
	if c, ok := t.(CachedTerrain); ok {
		sample = c.ElevationCachedM
	}
	if step < 1 {
		step = 1
	}
	w, h := v.Width/step, v.Height/step
	if w < 2 || h < 2 {
		return nil
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	// Sun from the north-west, the cartographic convention. It is a convention
	// rather than a physical claim, and getting it wrong inverts every valley
	// into a ridge for anyone used to a real map.
	sun := [3]float64{-0.6, 0.6, 0.53}

	heights := make([]float64, w*h)
	known := make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			lat, lon := v.ScreenToLatLon(float64(x*step), float64(y*step))
			hgt, ok := sample(lat, lon)
			heights[y*w+x], known[y*w+x] = hgt, ok
		}
	}

	scale := v.MetresPerPixel * float64(step)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if !known[i] {
				// Not sea level. Terrain nobody downloaded must not look like
				// terrain that is flat.
				img.Set(x, y, color.RGBA{R: 26, G: 27, B: 32, A: 255})
				continue
			}
			hx := sampleDelta(heights, known, w, h, x, y, 1, 0)
			hy := sampleDelta(heights, known, w, h, x, y, 0, 1)
			nx, ny, nz := -hx/scale, hy/scale, 1.0
			l := math.Sqrt(nx*nx + ny*ny + nz*nz)
			shade := (nx*sun[0] + ny*sun[1] + nz*sun[2]) / l
			shade = clampF(shade, 0.15, 1)

			// Altitude tints the base colour; the shading carries the shape.
			alt := clampF(heights[i]/900, 0, 1)
			r := 0.32 + 0.38*alt
			g := 0.38 + 0.30*alt
			b := 0.30 + 0.34*alt
			img.Set(x, y, color.RGBA{
				R: uint8(255 * clampF(r*shade, 0, 1)),
				G: uint8(255 * clampF(g*shade, 0, 1)),
				B: uint8(255 * clampF(b*shade, 0, 1)),
				A: 255,
			})
		}
	}
	return img
}

// sampleDelta is a central difference that does not step outside the image or
// into cells with no data.
func sampleDelta(h []float64, known []bool, w, ht, x, y, dx, dy int) float64 {
	a, aok := at(h, known, w, ht, x-dx, y-dy)
	b, bok := at(h, known, w, ht, x+dx, y+dy)
	if !aok || !bok {
		return 0
	}
	return (b - a) / 2
}

func at(h []float64, known []bool, w, ht, x, y int) (float64, bool) {
	if x < 0 || y < 0 || x >= w || y >= ht {
		return 0, false
	}
	i := y*w + x
	return h[i], known[i]
}

func clampF(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }
