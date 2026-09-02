// A path index: many paths against one area's buildings.
//
// A coverage raster asks for obstructions on a hundred thousand paths per
// station. Going to the tile store per path - even along a corridor - pays
// a mutex and a dedupe map every time, and twelve workers ground against
// one lock for minutes. The index queries the store ONCE for the whole
// area, buckets the footprints, and answers each path with a lock-free
// walk of the buckets its line actually crosses.
package environ

import (
	"math"
)

// pathBucketDeg is the index's cell size: about a kilometre, a few
// buildings per bucket in a town and none outside one.
const pathBucketDeg = 0.01

// PathIndex answers ObstructionsOnPath for paths inside the area it was
// built over. Read-only after construction; share it across workers freely.
type PathIndex struct {
	blds    []Building
	buckets map[[2]int][]int32
	// tall holds the same index again for the structures the price charges
	// wherever they stand, so the walks that skip the middle of a path still
	// find them. Kept separate because it is nearly always empty and an
	// empty map is a walk that need not happen at all.
	tall   map[[2]int][]int32
	ground Ground
}

// NewPathIndex collects and buckets everything standing in the box. The
// box must cover both ends of every path it will be asked about.
func NewPathIndex(p Provider, g Ground, minLat, minLon, maxLat, maxLon float64) *PathIndex {
	ix := &PathIndex{buckets: map[[2]int][]int32{}, tall: map[[2]int][]int32{}, ground: g}
	if p == nil {
		return ix
	}
	ix.blds = p.Buildings(minLat, minLon, maxLat, maxLon)
	for i, b := range ix.blds {
		if len(b.Footprint) == 0 {
			continue
		}
		bMinLat, bMaxLat := b.Footprint[0][0], b.Footprint[0][0]
		bMinLon, bMaxLon := b.Footprint[0][1], b.Footprint[0][1]
		for _, v := range b.Footprint[1:] {
			bMinLat = math.Min(bMinLat, v[0])
			bMaxLat = math.Max(bMaxLat, v[0])
			bMinLon = math.Min(bMinLon, v[1])
			bMaxLon = math.Max(bMaxLon, v[1])
		}
		x0 := int(math.Floor(bMinLon / pathBucketDeg))
		x1 := int(math.Floor(bMaxLon / pathBucketDeg))
		y0 := int(math.Floor(bMinLat / pathBucketDeg))
		y1 := int(math.Floor(bMaxLat / pathBucketDeg))
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				k := [2]int{x, y}
				ix.buckets[k] = append(ix.buckets[k], int32(i))
				if b.HeightM >= tallM {
					ix.tall[k] = append(ix.tall[k], int32(i))
				}
			}
		}
	}
	return ix
}

// Buildings is how many the index holds - zero means the area is bare.
func (ix *PathIndex) Buildings() int { return len(ix.blds) }

// PathScratch is one worker's reusable state: an epoch-marked seen set, so
// deduplication costs no allocation and no clearing between paths.
type PathScratch struct {
	seen  []uint32
	epoch uint32
}

// ObstructionsOnPath is the indexed twin of the package function: the same
// crossings, found by walking the line's own buckets.
func (ix *PathIndex) ObstructionsOnPath(sc *PathScratch,
	aLat, aLon, bLat, bLon float64) []Obstruction {
	if len(ix.blds) == 0 {
		return nil
	}
	ix.beginWalk(sc)
	return ix.appendSub(sc, nil, ix.buckets, aLat, aLon, bLat, bLon, 0, 1)
}

// beginWalk readies the scratch for a fresh set of walks that must
// deduplicate against each other.
func (ix *PathIndex) beginWalk(sc *PathScratch) {
	if len(sc.seen) < len(ix.blds) {
		sc.seen = make([]uint32, len(ix.blds))
	}
	sc.epoch++
}

// appendTall adds the structures the price charges wherever they stand,
// over the whole path. Almost always a no-op: a dataset with no heights of
// its own has none of them.
func (ix *PathIndex) appendTall(sc *PathScratch, obs []Obstruction,
	aLat, aLon, bLat, bLon float64) []Obstruction {
	if len(ix.tall) == 0 {
		return obs
	}
	return ix.appendSub(sc, obs, ix.tall, aLat, aLon, bLat, bLon, 0, 1)
}

// PathLossDB prices the indexed obstructions exactly as PathBuildingLossDB
// prices the store's.
func (ix *PathIndex) PathLossDB(sc *PathScratch,
	aLat, aLon, txAslM, bLat, bLon, rxAslM, totalM, freqMHz float64) float64 {
	if totalM <= 0 {
		return 0
	}
	obs := ix.ObstructionsOnPath(sc, aLat, aLon, bLat, bLon)
	return priceObstructions(obs, txAslM, rxAslM, totalM, freqMHz)
}

// PathLossNearEndsDB walks only the ends of the path for candidates, which
// is all a raster can afford: a hundred thousand full-path walks per station
// is what froze one. It is not a cheaper answer, it is the same answer -
// priceObstructions charges nothing beyond apertureM of either end, so the
// crossings this walk declines to look for are the ones the price would have
// discarded. Crossing fractions are computed against the FULL path, so the
// knife-edge arithmetic is exact for every building it does price.
func (ix *PathIndex) PathLossNearEndsDB(sc *PathScratch,
	aLat, aLon, txAslM, bLat, bLon, rxAslM, totalM, freqMHz float64) float64 {
	if totalM <= 0 || len(ix.blds) == 0 {
		return 0
	}
	if totalM <= 2*apertureM {
		return priceObstructions(
			ix.ObstructionsOnPath(sc, aLat, aLon, bLat, bLon),
			txAslM, rxAslM, totalM, freqMHz)
	}
	// One epoch across every window. The bucket walk reaches a step's
	// neighbours, so on a path a few buckets long the windows overlap, and
	// an epoch each would let a building in the overlap cross twice and be
	// charged as two screens.
	ix.beginWalk(sc)
	f := apertureM / totalM
	obs := ix.appendSub(sc, nil, ix.buckets, aLat, aLon, bLat, bLon, 0, f)
	obs = ix.appendSub(sc, obs, ix.buckets, aLat, aLon, bLat, bLon, 1-f, 1)
	obs = ix.appendTall(sc, obs, aLat, aLon, bLat, bLon)
	return priceObstructions(obs, txAslM, rxAslM, totalM, freqMHz)
}

// NearMask stamps, onto a raster grid, which cells have any building inside
// the priced aperture at all. The dataset is patches around a network's
// nodes; most of a national raster is countryside where the cell-end walk
// would find nothing after fifty bucket lookups, and a precomputed "nothing
// here" is how ninety percent of cells skip it outright. The radius is the
// aperture rather than a caller's choice, because a cell wrongly called far
// skips a walk whose crossings the price would have charged for.
func (ix *PathIndex) NearMask(south, north, west, east float64, w, h int) []bool {
	mask := make([]bool, w*h)
	if len(ix.blds) == 0 {
		return mask
	}
	midLat := (south + north) / 2
	rLat := apertureM / 111320.0
	rLon := apertureM / (111320.0 * math.Cos(midLat*math.Pi/180))
	for k := range ix.buckets {
		bS := float64(k[1]) * pathBucketDeg
		bW := float64(k[0]) * pathBucketDeg
		y0 := int((bS - rLat - south) / (north - south) * float64(h))
		y1 := int((bS + pathBucketDeg + rLat - south) / (north - south) * float64(h))
		x0 := int((bW - rLon - west) / (east - west) * float64(w))
		x1 := int((bW + pathBucketDeg + rLon - west) / (east - west) * float64(w))
		for y := max(0, y0); y <= min(h-1, y1); y++ {
			// The raster's rows count from the north; the arithmetic above
			// counts from the south, so flip.
			ry := h - 1 - y
			for x := max(0, x0); x <= min(w-1, x1); x++ {
				mask[ry*w+x] = true
			}
		}
	}
	return mask
}

// nearSectors is the angular resolution of a station's near-set. At 3 km a
// sector is ~37 m wide - one building - so a cell's ray tests the
// footprints that could actually stand in its way, not the whole town.
const nearSectors = 512

// StationPaths is one station's precomputed view of the index: every
// building within the priced aperture, bucketed by the azimuth range it
// subtends.
// Built once per station; a raster then asks it a hundred thousand times.
type StationPaths struct {
	ix       *PathIndex
	lat, lon float64
	cosLat   float64
	sectors  [][]int32
}

// Station precomputes the near-set around one transmitter.
func (ix *PathIndex) Station(lat, lon float64) *StationPaths {
	sp := &StationPaths{ix: ix, lat: lat, lon: lon,
		cosLat:  math.Cos(lat * math.Pi / 180),
		sectors: make([][]int32, nearSectors)}
	// A square of this half-width contains the circle of the same radius, so
	// no crossing inside the priced aperture escapes the sector index.
	rDeg := apertureM / 111320.0
	for i, b := range ix.blds {
		if len(b.Footprint) == 0 {
			continue
		}
		minAz, maxAz := math.Inf(1), math.Inf(-1)
		near := false
		for _, v := range b.Footprint {
			dLat := v[0] - lat
			dLon := (v[1] - lon) * sp.cosLat
			if math.Abs(dLat) <= rDeg && math.Abs(dLon) <= rDeg {
				near = true
			}
			az := math.Atan2(dLon, dLat)
			minAz = math.Min(minAz, az)
			maxAz = math.Max(maxAz, az)
		}
		if !near {
			continue
		}
		s0 := int((minAz + math.Pi) / (2 * math.Pi) * nearSectors)
		s1 := int((maxAz + math.Pi) / (2 * math.Pi) * nearSectors)
		if maxAz-minAz > math.Pi {
			// The footprint straddles the wrap; take the long way round
			// rather than the wrong short one.
			s0, s1 = s1, s0+nearSectors
		}
		for sct := s0 - 1; sct <= s1+1; sct++ {
			k := ((sct % nearSectors) + nearSectors) % nearSectors
			sp.sectors[k] = append(sp.sectors[k], int32(i))
		}
	}
	return sp
}

// LossDB prices the path from the station to one cell: the station-end
// candidates from the ray's own sector, the cell-end candidates from a
// short walk, every crossing priced against the full path.
func (sp *StationPaths) LossDB(sc *PathScratch, cellNear bool,
	txAslM, cellLat, cellLon, rxAslM, totalM, freqMHz float64) float64 {
	ix := sp.ix
	if totalM <= 0 || len(ix.blds) == 0 {
		return 0
	}
	ix.beginWalk(sc)
	az := math.Atan2((cellLon-sp.lon)*sp.cosLat, cellLat-sp.lat)
	sct := int((az+math.Pi)/(2*math.Pi)*nearSectors) % nearSectors
	var obs []Obstruction
	for _, bi := range sp.sectors[sct] {
		if sc.seen[bi] == sc.epoch {
			continue
		}
		sc.seen[bi] = sc.epoch
		obs = ix.appendCrossing(obs, bi, sp.lat, sp.lon, cellLat, cellLon)
	}
	// The cell end, when the path is long enough that the station's own
	// aperture cannot have covered it - and only when the mask says the
	// cell has anything within reach at all.
	if cellNear && totalM > apertureM {
		f := 1 - apertureM/totalM
		obs = ix.appendSub(sc, obs, ix.buckets, sp.lat, sp.lon, cellLat, cellLon, f, 1)
	}
	obs = ix.appendTall(sc, obs, sp.lat, sp.lon, cellLat, cellLon)
	return priceObstructions(obs, txAslM, rxAslM, totalM, freqMHz)
}

// appendCrossing tests one candidate against the full path.
func (ix *PathIndex) appendCrossing(obs []Obstruction, bi int32,
	aLat, aLon, bLat, bLon float64) []Obstruction {
	bl := &ix.blds[bi]
	enter, exit, crosses := segmentPolygon(aLat, aLon, bLat, bLon, bl.Footprint)
	if !crosses {
		return obs
	}
	midLat := aLat + (bLat-aLat)*(enter+exit)/2
	midLon := aLon + (bLon-aLon)*(enter+exit)/2
	ground := 0.0
	if ix.ground != nil {
		if h, ok := ix.ground.ElevationM(midLat, midLon); ok {
			ground = h
		}
	}
	return append(obs, Obstruction{
		EnterFrac: enter, ExitFrac: exit,
		TopM: ground + bl.HeightM, HeightM: bl.HeightM,
		Material: bl.Material, MaterialConfidence: bl.MaterialConfidence,
	})
}

// appendSub walks [f0,f1] of the path through one bucket map, under the
// caller's epoch.
func (ix *PathIndex) appendSub(sc *PathScratch, obs []Obstruction,
	buckets map[[2]int][]int32, aLat, aLon, bLat, bLon, f0, f1 float64) []Obstruction {
	dLat, dLon := bLat-aLat, bLon-aLon
	span := math.Max(math.Abs(dLat), math.Abs(dLon)) * (f1 - f0)
	steps := int(span/(pathBucketDeg/2)) + 1
	lastK := [2]int{1 << 30, 1 << 30}
	for i := 0; i <= steps; i++ {
		f := f0 + (f1-f0)*float64(i)/float64(steps)
		lat := aLat + dLat*f
		lon := aLon + dLon*f
		k := [2]int{int(math.Floor(lon / pathBucketDeg)), int(math.Floor(lat / pathBucketDeg))}
		if k == lastK {
			continue
		}
		lastK = k
		// The bucket and its neighbours: a footprint hugging a bucket edge
		// must not be missed by a line that runs just the other side of it.
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				for _, bi := range buckets[[2]int{k[0] + dx, k[1] + dy}] {
					if sc.seen[bi] == sc.epoch {
						continue
					}
					sc.seen[bi] = sc.epoch
					obs = ix.appendCrossing(obs, bi, aLat, aLon, bLat, bLon)
				}
			}
		}
	}
	return obs
}
