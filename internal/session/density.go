// Neighbour density as a vary axis (plan §6/11).
//
// The other vary axis, firmware version, changes what each node runs.
// Density changes where each node sits: node count, radio settings and the
// deployment area stay exactly as the loaded scenario has them, and only the
// arrangement changes, calibrated per arm so the mean node reaches the
// requested number of others.
//
// Density and margin are not independent - packing nodes into a smaller area
// raises neighbour count and shortens every link, so an arm that measured
// "more neighbours" would really be measuring "shorter links" too. This
// generator holds the deployment radius fixed across every density and
// varies only how many cluster centres the nodes are gathered around: fewer,
// tighter clusters raise neighbour count without changing the *scale* of the
// links that do close, because the spread within a cluster is a fixed
// fraction of the deployment radius regardless of cluster count.
package session

import (
	"math"

	"github.com/MeshBench/meshbench/internal/dsp"
	"github.com/MeshBench/meshbench/internal/linkbudget"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// clusterSpreadFrac is a cluster's scatter radius as a fraction of the whole
// deployment radius. Fixed regardless of density, which is what keeps a
// closing link's typical length - and so its margin - from drifting as the
// cluster count changes; only how many nodes share a cluster does.
const clusterSpreadFrac = 0.12

// PlaceForDensity returns base with every transmitting node repositioned so
// its mean neighbour count (an FSPL-only, terrain-free estimate - a fast
// stand-in for the real physics the engine will use once these positions are
// simulated, not a claim about what the engine will measure) is close to
// target. seed and target together determine the layout: the same pair
// always produces the same positions, and two different targets at the same
// seed produce different, not merely shifted, layouts.
//
// Observer and emitter nodes are left where they were - this shape is about
// how densely repeaters and companions sit among each other, not about
// where a fixed sensor happens to be.
func PlaceForDensity(base []scenario.Node, target float64, seed uint64) []scenario.Node {
	out := make([]scenario.Node, len(base))
	copy(out, base)
	if target <= 0 {
		return out
	}

	var idx []int
	for i, n := range out {
		if n.Kind.Transmits() {
			idx = append(idx, i)
		}
	}
	if len(idx) < 2 {
		return out
	}

	centreLat, centreLon, radiusKm := bounds(out, idx)
	if radiusKm <= 0 {
		radiusKm = 1
	}
	// Mixed into the RNG key, not added to the seed: two densities at one
	// seed must not land on the same stream at an offset, the same reasoning
	// dsp.Philox's own seed mixing is built on.
	rng := dsp.Philox{Seed: seed ^ uint64(math.Float64bits(target))*0x9E3779B97F4A7C15}

	place := func(k int) []scenario.Node {
		trial := make([]scenario.Node, len(out))
		copy(trial, out)
		centres := make([]scenario.LatLon, k)
		for c := 0; c < k; c++ {
			centres[c] = jitter(centreLat, centreLon, radiusKm, rng, uint64(c)*2)
		}
		for j, i := range idx {
			c := centres[j%k]
			p := jitter(c.Lat, c.Lon, radiusKm*clusterSpreadFrac, rng,
				uint64(len(centres))*2+uint64(j)*2)
			trial[i].Position = p
		}
		return trial
	}

	// K clusters, fewest first: K=1 packs every node into one cluster (the
	// highest density this generator can reach), K=len(idx) gives each node
	// its own (the lowest). Density falls as K rises, so a binary search on
	// K converges on the K whose measured mean neighbour count is closest to
	// target.
	lo, hi := 1, len(idx)
	bestK, bestDiff := lo, math.Inf(1)
	for lo <= hi {
		mid := (lo + hi) / 2
		trial := place(mid)
		got := meanNeighbours(trial, idx)
		if diff := math.Abs(got - target); diff < bestDiff {
			bestK, bestDiff = mid, diff
		}
		if got > target {
			lo = mid + 1 // too dense - spread across more clusters
		} else {
			hi = mid - 1 // too sparse - fewer clusters
		}
	}
	return place(bestK)
}

// bounds returns the centroid and the furthest any node sits from it, over
// the named indices - the deployment radius every density arm shares.
func bounds(nodes []scenario.Node, idx []int) (lat, lon, radiusKm float64) {
	for _, i := range idx {
		lat += nodes[i].Position.Lat
		lon += nodes[i].Position.Lon
	}
	lat /= float64(len(idx))
	lon /= float64(len(idx))
	for _, i := range idx {
		if d := distanceKm(lat, lon, nodes[i].Position.Lat, nodes[i].Position.Lon); d > radiusKm {
			radiusKm = d
		}
	}
	return lat, lon, radiusKm
}

// meanNeighbours is the average, over every transmitting node, of how many
// other transmitting nodes it has a positive one-way-worst margin to under
// free-space path loss alone - deliberately terrain-free, because this runs
// once per binary-search step to place nodes and needs to be fast, not
// exact; the engine's own real physics is what the resulting run measures.
func meanNeighbours(nodes []scenario.Node, idx []int) float64 {
	total := 0
	for _, i := range idx {
		for _, j := range idx {
			if i == j {
				continue
			}
			d := distanceKm(nodes[i].Position.Lat, nodes[i].Position.Lon,
				nodes[j].Position.Lat, nodes[j].Position.Lon)
			if d < 0.01 {
				d = 0.01
			}
			freqMHz := nodes[i].Radio.CentreHz / 1e6
			if freqMHz <= 0 {
				freqMHz = 869.618
			}
			fspl := 32.44 + 20*math.Log10(d) + 20*math.Log10(freqMHz)
			if linkbudget.MarginDB(nodes[i], nodes[j], fspl) > 0 {
				total++
			}
		}
	}
	return float64(total) / float64(len(idx))
}

// jitter picks a point uniformly inside a disc of radiusKm around (lat,lon),
// deterministic in (rng, counter).
func jitter(lat, lon, radiusKm float64, rng dsp.Philox, counter uint64) scenario.LatLon {
	u1 := float64(rng.Uint64At(counter)>>11) / float64(uint64(1)<<53)
	u2 := float64(rng.Uint64At(counter+1)>>11) / float64(uint64(1)<<53)
	r := radiusKm * math.Sqrt(u1)
	theta := 2 * math.Pi * u2
	const kmPerDegLat = 111.32
	dLat := (r * math.Cos(theta)) / kmPerDegLat
	dLon := (r * math.Sin(theta)) / (kmPerDegLat * math.Cos(lat*math.Pi/180))
	return scenario.LatLon{Lat: lat + dLat, Lon: lon + dLon}
}

func distanceKm(lat1, lon1, lat2, lon2 float64) float64 {
	const rEarth = 6371.0
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	s := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * rEarth * math.Asin(math.Min(1, math.Sqrt(s)))
}
