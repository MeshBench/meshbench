package terrain

import (
	"context"
	"math"
	"os"
)

// ElevationM samples the DEM, bilinearly.
//
// Bilinear rather than nearest-neighbour because a path profile walks across a
// tile in steps far smaller than a pixel, and nearest-neighbour turns a smooth
// hillside into a staircase. The steps are then read as a row of knife edges by
// the diffraction model, which invents obstruction that is not there.
//
// The bool is false where no tile covers the point, and that is not the same
// as sea level: a raster over a coastline asks for points outside its
// downloaded area, and answering zero draws confident coverage over the
// Atlantic.
func (s *TileStore) ElevationM(lat, lon float64) (float64, bool) {
	return s.ElevationAt(context.Background(), lat, lon)
}

// ElevationAt is ElevationM with a context, for the download path.
func (s *TileStore) ElevationAt(ctx context.Context, lat, lon float64) (float64, bool) {
	if lat > 85.0511 || lat < -85.0511 {
		return 0, false // outside the Web Mercator domain the tiles are cut on
	}
	zoom := s.zoom()
	n := math.Exp2(float64(zoom))

	// Fractional pixel position in the global tile grid.
	px := (lon + 180) / 360 * n * tileSize
	latRad := lat * math.Pi / 180
	py := (1 - math.Log(math.Tan(latRad)+1/math.Cos(latRad))/math.Pi) / 2 * n * tileSize

	x0f, y0f := math.Floor(px-0.5), math.Floor(py-0.5)
	fx, fy := px-0.5-x0f, py-0.5-y0f

	var corners [4]float64
	for i, d := range [4][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
		h, ok := s.pixel(ctx, int(x0f)+d[0], int(y0f)+d[1], zoom)
		if !ok {
			return 0, false
		}
		corners[i] = h
	}
	top := corners[0]*(1-fx) + corners[1]*fx
	bottom := corners[2]*(1-fx) + corners[3]*fx
	return top*(1-fy) + bottom*fy, true
}

// pixel reads one global-grid pixel, fetching its tile if needed.
func (s *TileStore) pixel(ctx context.Context, gx, gy, zoom int) (float64, bool) {
	maxPixel := int(math.Exp2(float64(zoom))) * tileSize
	if gy < 0 || gy >= maxPixel {
		return 0, false
	}
	// Longitude wraps; latitude does not.
	gx = ((gx % maxPixel) + maxPixel) % maxPixel

	tx, ty := gx/tileSize, gy/tileSize
	t, err := s.get(ctx, tx, ty)
	if err != nil {
		return 0, false
	}
	return float64(t.heights[(gy%tileSize)*tileSize+(gx%tileSize)]), true
}

// ElevationCachedM answers only from what is already downloaded.
//
// Drawing must never block on a network fetch. A map that samples the DEM a
// hundred thousand times per redraw and hits an uncached tile stops painting
// entirely, and the window that results is indistinguishable from a crash —
// which is exactly what it looked like the first time.
//
// The distinction is not "offline mode": downloads may well be enabled, and the
// operator may be prefetching in the background. It is that *this* caller
// cannot wait, and would rather draw a gap.
func (s *TileStore) ElevationCachedM(lat, lon float64) (float64, bool) {
	if lat > 85.0511 || lat < -85.0511 {
		return 0, false
	}
	zoom := s.zoom()
	n := math.Exp2(float64(zoom))
	px := (lon + 180) / 360 * n * tileSize
	latRad := lat * math.Pi / 180
	py := (1 - math.Log(math.Tan(latRad)+1/math.Cos(latRad))/math.Pi) / 2 * n * tileSize

	x0f, y0f := math.Floor(px-0.5), math.Floor(py-0.5)
	fx, fy := px-0.5-x0f, py-0.5-y0f

	var c [4]float64
	for i, d := range [4][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
		h, ok := s.cachedPixel(int(x0f)+d[0], int(y0f)+d[1], zoom)
		if !ok {
			return 0, false
		}
		c[i] = h
	}
	top := c[0]*(1-fx) + c[1]*fx
	bottom := c[2]*(1-fx) + c[3]*fx
	return top*(1-fy) + bottom*fy, true
}

// cachedPixel reads a pixel without touching the network.
//
// Memory first, then disk. Checking memory alone was wrong in a way that looked
// like a missing download: a freshly started process has an empty in-memory map,
// so the map drew gaps over tiles that were sitting on disk, and pressing
// "get terrain" appeared to do nothing because they were already there.
//
// Disk is allowed because the renderer runs off the frame thread. The network
// is not, at any distance.
func (s *TileStore) cachedPixel(gx, gy, zoom int) (float64, bool) {
	maxPixel := int(math.Exp2(float64(zoom))) * tileSize
	if gy < 0 || gy >= maxPixel {
		return 0, false
	}
	gx = ((gx % maxPixel) + maxPixel) % maxPixel
	tx, ty := gx/tileSize, gy/tileSize
	k := s.key(tx, ty)

	s.mu.RLock()
	t, ok := s.loaded[k]
	s.mu.RUnlock()

	if !ok {
		b, err := os.ReadFile(s.path(tx, ty))
		if err != nil {
			return 0, false
		}
		decoded, err := decodeTerrarium(b)
		if err != nil {
			return 0, false
		}
		s.mu.Lock()
		if existing, raced := s.loaded[k]; raced {
			decoded = existing
		} else {
			s.loaded[k] = decoded
		}
		s.mu.Unlock()
		t = decoded
	}
	return float64(t.heights[(gy%tileSize)*tileSize+(gx%tileSize)]), true
}
