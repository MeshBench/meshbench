package coverage

import "math"

// HeightGrid is the terrain rasterised once for one coverage area.
//
// The DEM lookups are the expensive part of a coverage raster — every cell of
// every station walks a profile of them. Sampling the ground once into a flat
// array turns forty stations' worth of repeated tile reads into forty passes
// over memory, and it is the form a GPU kernel can consume at all.
type HeightGrid struct {
	South, North, West, East float64
	W, H                     int
	// Heights is row-major, y=0 at the south edge. NoDataHeight marks a cell
	// the terrain could not answer for — a sentinel rather than NaN, because
	// NaN comparisons are exactly what a GPU's fast-math mode is allowed to
	// mangle, and a mangled no-data check produces plausible ground.
	Heights []float32
}

// NoDataHeight marks an unanswered grid cell.
const NoDataHeight = float32(-1e30)

// RasteriseHeights samples the terrain into a grid. The second return is the
// fraction of cells the terrain could answer for — a grid mostly NaN means
// the tiles are not downloaded, and the caller should say so rather than
// produce a raster of holes.
func RasteriseHeights(t Terrain, south, north, west, east float64, w, h int) (HeightGrid, float64) {
	g := HeightGrid{South: south, North: north, West: west, East: east, W: w, H: h,
		Heights: make([]float32, w*h)}
	known := 0
	for y := 0; y < h; y++ {
		lat := south + (north-south)*(float64(y)+0.5)/float64(h)
		for x := 0; x < w; x++ {
			lon := west + (east-west)*(float64(x)+0.5)/float64(w)
			if m, ok := t.ElevationM(lat, lon); ok {
				g.Heights[y*w+x] = float32(m)
				known++
			} else {
				g.Heights[y*w+x] = NoDataHeight
			}
		}
	}
	return g, float64(known) / float64(w*h)
}

// At samples the grid bilinearly. The same interpolation the WGSL kernel
// performs, so the CPU twin and the GPU read the same ground.
func (g HeightGrid) At(lat, lon float64) (float64, bool) {
	fx := (lon - g.West) / (g.East - g.West) * float64(g.W)
	fy := (lat - g.South) / (g.North - g.South) * float64(g.H)
	fx -= 0.5
	fy -= 0.5
	x0 := int(math.Floor(fx))
	y0 := int(math.Floor(fy))
	tx := fx - float64(x0)
	ty := fy - float64(y0)
	clampI := func(v, hi int) int {
		if v < 0 {
			return 0
		}
		if v > hi {
			return hi
		}
		return v
	}
	x0c, x1c := clampI(x0, g.W-1), clampI(x0+1, g.W-1)
	y0c, y1c := clampI(y0, g.H-1), clampI(y0+1, g.H-1)
	h00 := float64(g.Heights[y0c*g.W+x0c])
	h10 := float64(g.Heights[y0c*g.W+x1c])
	h01 := float64(g.Heights[y1c*g.W+x0c])
	h11 := float64(g.Heights[y1c*g.W+x1c])
	if h00 <= float64(NoDataHeight)/2 || h10 <= float64(NoDataHeight)/2 ||
		h01 <= float64(NoDataHeight)/2 || h11 <= float64(NoDataHeight)/2 {
		return 0, false
	}
	top := h00*(1-tx) + h10*tx
	bot := h01*(1-tx) + h11*tx
	return top*(1-ty) + bot*ty, true
}

// GridLossParams is everything one station's loss raster depends on — one
// struct, because the CPU twin and the GPU kernel must be fed identically or
// the equivalence test measures the plumbing instead of the maths.
type GridLossParams struct {
	// Station position and its antenna altitude above sea level.
	StLat, StLon, StAltM float64
	// The output raster's geometry.
	RasterW, RasterH         int
	South, North, West, East float64
	RemoteHeightM, FreqMHz   float64
	// Steps along each profile. Fixed rather than per-distance so every cell
	// costs the same on the GPU and the two implementations agree exactly.
	Steps int
}

// noDataLoss marks a cell the terrain could not answer for.
const noDataLoss = float32(math.MaxFloat32)

// GridLossCPU is the CPU twin of the GPU coverage kernel (ADR-0004: every
// kernel has one, and they are tested against each other).
//
// It is the same Bullington construction as terrain.MultiEdgeLossDB, over the
// rasterised grid instead of the DEM. It is *not* a fallback for the tile
// path — coverage.Compute stays the reference for small rasters; this exists
// to be exactly what the GPU computes, slower.
func GridLossCPU(g HeightGrid, p GridLossParams) []float32 {
	out := make([]float32, p.RasterW*p.RasterH)
	for y := 0; y < p.RasterH; y++ {
		for x := 0; x < p.RasterW; x++ {
			lat := p.South + (p.North-p.South)*(float64(y)+0.5)/float64(p.RasterH)
			lon := p.West + (p.East-p.West)*(float64(x)+0.5)/float64(p.RasterW)
			out[y*p.RasterW+x] = gridLossOne(g, p, lat, lon)
		}
	}
	return out
}

// gridLossOne is one cell: FSPL plus Bullington diffraction over the grid.
func gridLossOne(g HeightGrid, p GridLossParams, lat, lon float64) float32 {
	distKm := haversineKm(p.StLat, p.StLon, lat, lon)
	if distKm <= 0 {
		return 0
	}
	remoteGround, ok := g.At(lat, lon)
	if !ok {
		return noDataLoss
	}
	d := distKm * 1000
	lambda := 299.792458 / p.FreqMHz
	txAlt := p.StAltM
	rxAlt := remoteGround + p.RemoteHeightM
	slope := (rxAlt - txAlt) / d

	// Steepest sight line from each end, over the bulged profile — the same
	// single pass MultiEdgeLossDB makes, with the grid supplying the ground.
	maxFromTx, maxFromRx := -math.MaxFloat64, -math.MaxFloat64
	worstV := -math.MaxFloat64
	seen := false
	for i := 1; i < p.Steps; i++ {
		f := float64(i) / float64(p.Steps)
		h, ok := g.At(p.StLat+(lat-p.StLat)*f, p.StLon+(lon-p.StLon)*f)
		if !ok {
			return noDataLoss
		}
		d1 := d * f
		d2 := d - d1
		hb := h + d1*d2/(2*4.0/3.0*6_371_000)
		if s := (hb - txAlt) / d1; s > maxFromTx {
			maxFromTx = s
		}
		if s := (hb - rxAlt) / d2; s > maxFromRx {
			maxFromRx = s
		}
		hLos := hb - (txAlt + slope*d1)
		if v := hLos * math.Sqrt(2*d/(lambda*d1*d2)); v > worstV {
			worstV = v
		}
		seen = true
	}
	fspl := 32.44 + 20*math.Log10(distKm) + 20*math.Log10(p.FreqMHz)
	if !seen {
		return float32(fspl)
	}

	var v float64
	if maxFromTx < slope {
		v = worstV
	} else {
		db := (rxAlt - txAlt + maxFromRx*d) / (maxFromTx + maxFromRx)
		if db <= 0 || db >= d {
			return float32(fspl)
		}
		h := txAlt + maxFromTx*db - (txAlt + slope*db)
		v = h * math.Sqrt(2*d/(lambda*db*(d-db)))
	}
	loss := 0.0
	if v > -0.78 {
		loss = 6.9 + 20*math.Log10(math.Sqrt((v-0.1)*(v-0.1)+1)+v-0.1)
	}
	if loss > 0 {
		loss += (1 - math.Exp(-loss/6)) * (10 + 0.02*d/1000)
	}
	return float32(fspl + loss)
}
