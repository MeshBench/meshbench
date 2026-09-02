// Package environ is what physically stands in the way: buildings, with
// heights, materials and the honesty about how well any of it is known.
//
// The core principle is the global RF environment plan's, and it is the same
// one the whole simulator lives by: the dataset describes what exists; the
// RF engine decides what that does to a signal. Nothing in these tiles is a
// decibel.
//
// Storage is tile-per-file, loaded on demand and cached, beside the terrain
// tiles it complements. The format is gzipped JSON lines rather than the
// plan's GeoParquet - one fewer dependency, streams fine at tile size, and
// the upgrade is recorded in the plan if the datasets outgrow it.
package environ

import (
	"math"
)

// Building is one footprint: a polygon on the ground and a height above it.
type Building struct {
	// Footprint is lat/lon vertices, closed implicitly (last connects to
	// first). 2D plus a height, as the plan prescribes - a mesh is generated
	// when someone needs one, not stored.
	Footprint [][2]float64 `json:"fp"`
	// HeightM is above ground; the terrain supplies the ground.
	HeightM float64 `json:"h"`
	// HeightSource and HeightConfidence say where the number came from -
	// "microsoft", "osm", "levels", "default" - and how much to trust it.
	HeightSource     string  `json:"hs,omitempty"`
	HeightConfidence float64 `json:"hc,omitempty"`
	// Material is the MeshBench taxonomy value, with its own provenance.
	Material           string  `json:"m,omitempty"`
	MaterialSource     string  `json:"ms,omitempty"`
	MaterialConfidence float64 `json:"mc,omitempty"`
	// Type is the source's classification - residential, industrial - kept
	// because material inference reads it.
	Type string `json:"t,omitempty"`
}

// Provider answers what stands inside a bounding box. Implementations load
// tiles; tests hand back fixtures.
type Provider interface {
	// Buildings returns everything intersecting the box. Missing coverage
	// returns an empty slice and Missing() counts it - absence of data and
	// absence of buildings must not look the same to a caller who asks.
	Buildings(minLat, minLon, maxLat, maxLon float64) []Building
}

// Obstruction is one building crossed by a path, in path terms.
type Obstruction struct {
	// EnterFrac and ExitFrac are fractions along the path where the ray is
	// inside the footprint.
	EnterFrac, ExitFrac float64
	// TopM is ground plus building height at the crossing, above sea level.
	TopM float64
	// HeightM is the building's own height above the ground it stands on,
	// which decides whether the price treats it as a rooftop or as a
	// structure in the way.
	HeightM float64
	// Material carries the through-loss question to the RF engine.
	Material           string
	MaterialConfidence float64
}

// Ground answers terrain height, so obstruction tops are absolute.
type Ground interface {
	ElevationM(lat, lon float64) (float64, bool)
}

// ObstructionsOnPath is every building the straight path from a to b passes
// through, with entry and exit as fractions of the path.
//
// Candidates come from a corridor walked ALONG the path, one tile-sized
// step at a time, not from the path's bounding box: a 50 km diagonal's box
// is a rectangle holding whole towns, and a coverage raster asking for a
// hundred thousand paths against every building in that rectangle froze a
// run solid. The corridor holds a few hundred candidates where the box
// held tens of thousands, and the same buildings cross either way.
func ObstructionsOnPath(p Provider, g Ground, aLat, aLon, bLat, bLon float64) []Obstruction {
	if p == nil {
		return nil
	}
	var out []Obstruction
	for _, bl := range corridorBuildings(p, aLat, aLon, bLat, bLon) {
		enter, exit, crosses := segmentPolygon(aLat, aLon, bLat, bLon, bl.Footprint)
		if !crosses {
			continue
		}
		midLat := aLat + (bLat-aLat)*(enter+exit)/2
		midLon := aLon + (bLon-aLon)*(enter+exit)/2
		ground := 0.0
		if g != nil {
			if h, ok := g.ElevationM(midLat, midLon); ok {
				ground = h
			}
		}
		out = append(out, Obstruction{
			EnterFrac: enter, ExitFrac: exit,
			TopM: ground + bl.HeightM, HeightM: bl.HeightM,
			Material: bl.Material, MaterialConfidence: bl.MaterialConfidence,
		})
	}
	return out
}

// corridorStepDeg is the corridor's sampling step and half-width, a shade
// over one z14 tile so no building beside the line is missed.
const corridorStepDeg = 0.012

// corridorBuildings collects the candidates along the path, deduplicated -
// neighbouring steps share tiles, and a building must cross once, not once
// per step that saw it.
func corridorBuildings(p Provider, aLat, aLon, bLat, bLon float64) []Building {
	dLat, dLon := bLat-aLat, bLon-aLon
	span := math.Max(math.Abs(dLat), math.Abs(dLon))
	steps := int(span/corridorStepDeg) + 1
	seen := map[[2]float64]bool{}
	var out []Building
	for i := 0; i <= steps; i++ {
		f := float64(i) / float64(steps)
		lat := aLat + dLat*f
		lon := aLon + dLon*f
		for _, bl := range p.Buildings(lat-corridorStepDeg, lon-corridorStepDeg,
			lat+corridorStepDeg, lon+corridorStepDeg) {
			if len(bl.Footprint) == 0 {
				continue
			}
			k := bl.Footprint[0]
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, bl)
		}
	}
	return out
}

// segmentPolygon finds where the segment a->b is inside the polygon, as
// entry and exit fractions. Points inside count from zero; a segment that
// only grazes a vertex is treated as missing, which for RF is the honest
// call at footprint precision.
func segmentPolygon(aLat, aLon, bLat, bLon float64, poly [][2]float64) (enter, exit float64, crosses bool) {
	if len(poly) < 3 {
		return 0, 0, false
	}
	var ts []float64
	if pointInPolygon(aLat, aLon, poly) {
		ts = append(ts, 0)
	}
	for i := range poly {
		j := (i + 1) % len(poly)
		if t, ok := segmentIntersectT(aLat, aLon, bLat, bLon,
			poly[i][0], poly[i][1], poly[j][0], poly[j][1]); ok {
			ts = append(ts, t)
		}
	}
	if pointInPolygon(bLat, bLon, poly) {
		ts = append(ts, 1)
	}
	if len(ts) < 2 {
		return 0, 0, false
	}
	lo, hi := ts[0], ts[0]
	for _, t := range ts[1:] {
		lo, hi = math.Min(lo, t), math.Max(hi, t)
	}
	if hi-lo < 1e-9 {
		return 0, 0, false
	}
	return lo, hi, true
}

// segmentIntersectT is where segment p meets segment q, as p's fraction.
func segmentIntersectT(p0a, p0b, p1a, p1b, q0a, q0b, q1a, q1b float64) (float64, bool) {
	ra, rb := p1a-p0a, p1b-p0b
	sa, sb := q1a-q0a, q1b-q0b
	den := ra*sb - rb*sa
	if math.Abs(den) < 1e-15 {
		return 0, false
	}
	t := ((q0a-p0a)*sb - (q0b-p0b)*sa) / den
	u := ((q0a-p0a)*rb - (q0b-p0b)*ra) / den
	if t < 0 || t > 1 || u < 0 || u > 1 {
		return 0, false
	}
	return t, true
}

func pointInPolygon(lat, lon float64, poly [][2]float64) bool {
	in := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		yi, xi := poly[i][0], poly[i][1]
		yj, xj := poly[j][0], poly[j][1]
		if (yi > lat) != (yj > lat) &&
			lon < (xj-xi)*(lat-yi)/(yj-yi)+xi {
			in = !in
		}
	}
	return in
}
