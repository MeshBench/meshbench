// Package scenario holds the things a study is defined by: nodes, region, seed.
package scenario

import "math"

// LatLon is a WGS84 position.
type LatLon struct{ Lat, Lon float64 }

// Ring is a closed polygon boundary, first point need not repeat the last.
type Ring []LatLon

// Boundary is one named area, as fetched from a provider (ADR-0019).
type Boundary struct {
	Name    string
	Source  string // "natural-earth", "ons", "osm", "drawn"
	Vintage string // which edition — administrative boundaries change
	Rings   []Ring // outer rings; multipolygons have several

	// Holes are interior rings: lochs, enclaves, and the hole in the middle of
	// a district that contains a city. A point inside a hole is outside the
	// boundary, and treating holes as extra outers includes precisely the area
	// that was meant to be excluded.
	Holes []Ring
}

// covers reports whether p is inside this boundary, holes taken out.
func (b Boundary) covers(p LatLon) bool {
	inside := false
	for _, ring := range b.Rings {
		if ring.contains(p) {
			inside = true
			break
		}
	}
	if !inside {
		return false
	}
	for _, hole := range b.Holes {
		if hole.contains(p) {
			return false
		}
	}
	return true
}

// Region is the union of several boundaries plus an RF margin.
//
// A set, not a single polygon, because ADR-0019 makes multi-select the normal
// case. Every consumer — node import, terrain prefetch, emitters — takes the
// union, so the boundary means one thing everywhere.
type Region struct {
	Boundaries []Boundary
	// MarginKm is how far outside the boundary external nodes and emitters are
	// still loaded as RF participants. A repeater just outside still interferes
	// with, and relays to, nodes inside — dropping it silently produces a mesh
	// that behaves better than reality.
	MarginKm float64
}

// DefaultMarginKm is beyond plausible LoRa reach, so nothing that could
// realistically participate is excluded.
const DefaultMarginKm = 30

// Contains reports whether p is inside any boundary — the study area proper.
func (r Region) Contains(p LatLon) bool {
	for _, b := range r.Boundaries {
		if b.covers(p) {
			return true
		}
	}
	return false
}

// Participates reports whether p should be simulated at all: inside the region,
// or within the RF margin of it.
//
// The distinction from Contains is the whole point of having a margin. Results
// are reported for what Contains; the RF is computed over what Participates.
func (r Region) Participates(p LatLon) bool {
	if r.Contains(p) {
		return true
	}
	margin := r.MarginKm
	if margin <= 0 {
		margin = DefaultMarginKm
	}
	for _, b := range r.Boundaries {
		for _, ring := range b.Rings {
			if ring.withinKm(p, margin) {
				return true
			}
		}
	}
	return false
}

// Bounds returns the bounding box of the region including its margin — what the
// terrain prefetch estimate is computed from (MSIM-38).
func (r Region) Bounds() (south, north, west, east float64) {
	south, west = 90, 180
	north, east = -90, -180
	for _, b := range r.Boundaries {
		for _, ring := range b.Rings {
			for _, p := range ring {
				south = math.Min(south, p.Lat)
				north = math.Max(north, p.Lat)
				west = math.Min(west, p.Lon)
				east = math.Max(east, p.Lon)
			}
		}
	}
	margin := r.MarginKm
	if margin <= 0 {
		margin = DefaultMarginKm
	}
	dLat := margin / 111.32
	mid := (south + north) / 2
	dLon := margin / (111.32 * math.Max(math.Cos(mid*math.Pi/180), 0.01))
	return south - dLat, north + dLat, west - dLon, east + dLon
}

// contains is the standard ray-casting test.
func (ring Ring) contains(p LatLon) bool {
	in := false
	n := len(ring)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		if (ring[i].Lat > p.Lat) != (ring[j].Lat > p.Lat) {
			x := (ring[j].Lon-ring[i].Lon)*(p.Lat-ring[i].Lat)/(ring[j].Lat-ring[i].Lat) + ring[i].Lon
			if p.Lon < x {
				in = !in
			}
		}
	}
	return in
}

// withinKm reports whether p is within d km of any edge of the ring.
func (ring Ring) withinKm(p LatLon, d float64) bool {
	for i := range ring {
		if haversineKm(p, ring[i]) <= d {
			return true
		}
	}
	return false
}

func haversineKm(a, b LatLon) float64 {
	const R = 6371.0
	la1, la2 := a.Lat*math.Pi/180, b.Lat*math.Pi/180
	dLa := (b.Lat - a.Lat) * math.Pi / 180
	dLo := (b.Lon - a.Lon) * math.Pi / 180
	h := math.Sin(dLa/2)*math.Sin(dLa/2) + math.Cos(la1)*math.Cos(la2)*math.Sin(dLo/2)*math.Sin(dLo/2)
	return 2 * R * math.Asin(math.Sqrt(h))
}
