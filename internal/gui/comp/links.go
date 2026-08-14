package comp

import (
	"math"

	"github.com/MeshBench/meshbench/internal/gui/state"
)

// Which nodes can hear which, cached.
//
// This is a proximity test over every pair, so it costs n squared haversines:
// 48,000 of them on the 311 node fixture. Doing that once is fine. Doing it on
// every frame was 43% of the map's CPU profile, and it was recomputing an
// answer that cannot have changed, because a node does not move between one
// frame and the next.
//
// So key the cache on the positions themselves rather than on the snapshot
// sequence, which increases on every tick of simulated time and would throw
// the cache away sixty times a second for a network that is standing still.

type linkCache struct {
	fp    uint64
	pairs [][2]int
}

// pairs returns the linked index pairs for these points, rebuilding only when
// the positions differ from the ones the cache was built from.
func (c *linkCache) get(pts []projected) [][2]int {
	fp := fingerprint(pts)
	if fp == c.fp && c.pairs != nil {
		return c.pairs
	}
	pairs := make([][2]int, 0, len(pts)*4)
	for i := range pts {
		for j := i + 1; j < len(pts); j++ {
			if near(pts[i].n, pts[j].n) {
				pairs = append(pairs, [2]int{i, j})
			}
		}
	}
	c.fp, c.pairs = fp, pairs
	return pairs
}

// fingerprint is an FNV-1a over the positions, in order.
//
// Cheap enough to run every frame - 311 nodes, two adds and two multiplies
// each - and it changes if a node moves, is added, is removed or is reordered,
// which is every way the answer can go stale.
func fingerprint(pts []projected) uint64 {
	const (
		offset = 14695981039346656037
		prime  = 1099511628211
	)
	h := uint64(offset)
	mix := func(v uint64) {
		for i := 0; i < 8; i++ {
			h ^= v & 0xff
			h *= prime
			v >>= 8
		}
	}
	for i := range pts {
		mix(math.Float64bits(pts[i].n.Lat))
		mix(math.Float64bits(pts[i].n.Lon))
	}
	mix(uint64(len(pts)))
	return h
}

// near is the same proximity rule the rest of the tool uses for a quick link
// estimate: close enough to hear on this band.
func near(a, b *state.Node) bool {
	const r = 6371.0
	dLat := (b.Lat - a.Lat) * math.Pi / 180
	dLon := (b.Lon - a.Lon) * math.Pi / 180
	la, lb := a.Lat*math.Pi/180, b.Lat*math.Pi/180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(la)*math.Cos(lb)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2*r*math.Asin(math.Sqrt(h)) < 18
}
