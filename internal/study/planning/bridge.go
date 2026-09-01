package planning

import (
	"fmt"
	"math"
	"sort"

	"github.com/MeshBench/meshbench/internal/rf/geo"
)

// Site is a place a node could go.
type Site struct {
	Lat, Lon   float64
	GroundM    float64
	HeightAGLm float64
	// Existing marks a site that is already built. The whole value of a bridge
	// search is that it reuses these for free, so they are never counted as a
	// cost.
	Existing bool
	Name     string

	// UncertaintyKm is the radius the position is good to. Zero on anything
	// this package proposes - high ground is chosen, not observed - so it only
	// ever carries a value on an existing site, from whatever imported it.
	UncertaintyKm float64
}

// LinkChecker decides whether two sites can work.
//
// An interface because the answer comes from terrain and a link budget in
// production and from a stated geometry in a test, and because a caller may
// want to substitute observed reach data for the model — which is HopReach's
// "terrain, observed, or a blend" choice, and the right one to offer.
type LinkChecker interface {
	// Works reports whether a two-way link closes between two sites.
	Works(a, b Site) bool
}

// Terrain supplies ground elevation for candidate generation.
type Terrain interface {
	ElevationM(lat, lon float64) (float64, bool)
}

// BridgeOptions control a search.
type BridgeOptions struct {
	// Existing sites are reused at no cost.
	Existing []Site

	// MastHeightM is what a new site would be built at.
	MastHeightM float64

	// MaxNew caps how many new sites a route may use. A search that will
	// happily propose twenty relays has not answered the question anybody
	// asked.
	MaxNew int

	// CandidateStep is the grid spacing in degrees for finding high ground.
	// Coarse on purpose: the point is to find hills, and a fine grid finds the
	// same hill many times over while taking far longer.
	CandidateStep float64

	// Alternatives is how many distinct routes to return.
	Alternatives int
}

// Route is one way to connect two points.
type Route struct {
	// Sites in order from one end to the other, including both endpoints.
	Sites []Site
	// NewSites is how many of them would have to be built.
	NewSites int
	// LongestHopKm is the weakest part of the chain, and usually the first
	// thing anyone wants to argue with.
	LongestHopKm float64

	// UncertainSites counts sites in the chain whose position is not surveyed,
	// and WorstUncertaintyKm is how loose the loosest of them is. Both, because
	// "one uncertain site" says nothing until you know whether that is 200 m or
	// 50 km. Every hop length in a route - and so whether the chain closes at
	// all - is only as good as the worst position in it, and a route is a plan
	// to spend money on masts.
	UncertainSites     int
	WorstUncertaintyKm float64
}

// Bridge finds the fewest new sites needed to connect two points.
//
// Fewest *new* sites, not fewest hops. Existing infrastructure is free, and a
// five-hop route over four repeaters that already exist is a better answer than
// a three-hop route needing two new masts — which is the opposite of what a
// plain shortest-path search returns.
func Bridge(from, to Site, t Terrain, check LinkChecker, o BridgeOptions) ([]Route, error) {
	if o.MastHeightM <= 0 {
		o.MastHeightM = 10
	}
	if o.MaxNew <= 0 {
		o.MaxNew = 4
	}
	if o.CandidateStep <= 0 {
		o.CandidateStep = 0.01
	}
	if o.Alternatives <= 0 {
		o.Alternatives = 3
	}

	// Nodes: the two ends, everything already built, and high ground between.
	nodes := []Site{from}
	nodes = append(nodes, o.Existing...)
	candidates, err := highGround(from, to, t, o)
	if err != nil {
		return nil, err
	}
	nodes = append(nodes, candidates...)
	nodes = append(nodes, to)
	toIdx := len(nodes) - 1

	// Dijkstra with cost = number of new sites. Existing ones cost nothing, so
	// a path through them is preferred however long it is.
	const inf = math.MaxInt32
	cost := make([]int, len(nodes))
	prev := make([]int, len(nodes))
	done := make([]bool, len(nodes))
	for i := range cost {
		cost[i], prev[i] = inf, -1
	}
	cost[0] = 0

	for {
		best := -1
		for i := range nodes {
			if !done[i] && cost[i] < inf && (best < 0 || cost[i] < cost[best]) {
				best = i
			}
		}
		if best < 0 {
			break
		}
		done[best] = true
		for j := range nodes {
			if done[j] || !check.Works(nodes[best], nodes[j]) {
				continue
			}
			step := 0
			if !nodes[j].Existing && j != toIdx {
				step = 1
			}
			if cost[best]+step < cost[j] {
				cost[j] = cost[best] + step
				prev[j] = best
			}
		}
	}

	if cost[toIdx] >= inf {
		return nil, fmt.Errorf("planning: no route found using up to %d new sites; "+
			"the gap may need a taller mast or a site outside the searched area", o.MaxNew)
	}
	if cost[toIdx] > o.MaxNew {
		return nil, fmt.Errorf("planning: the best route needs %d new sites, more than the %d allowed",
			cost[toIdx], o.MaxNew)
	}

	route := buildRoute(nodes, prev, toIdx)
	routes := []Route{route}

	// Alternatives, by forbidding one new site from the best route at a time.
	// Crude, and it produces genuinely different answers rather than
	// permutations of the same one, which is what someone comparing options
	// actually wants.
	for _, banned := range route.Sites {
		if len(routes) >= o.Alternatives {
			break
		}
		if banned.Existing || sameSite(banned, from) || sameSite(banned, to) {
			continue
		}
		alt, err := bridgeExcluding(nodes, check, banned)
		if err != nil {
			continue
		}
		if !duplicateRoute(routes, alt) {
			routes = append(routes, alt)
		}
	}
	sort.SliceStable(routes, func(i, j int) bool { return routes[i].NewSites < routes[j].NewSites })
	return routes, nil
}

// bridgeExcluding reruns the search with one site removed.
//
// Over the sites already found rather than through Bridge, which would
// regenerate candidates and recurse.
func bridgeExcluding(nodes []Site, check LinkChecker, banned Site) (Route, error) {
	filtered := make([]Site, 0, len(nodes))
	for _, n := range nodes {
		if sameSite(n, banned) {
			continue
		}
		filtered = append(filtered, n)
	}
	return searchRoute(filtered, check, len(filtered)-1)
}

func searchRoute(nodes []Site, check LinkChecker, toIdx int) (Route, error) {
	const inf = math.MaxInt32
	cost := make([]int, len(nodes))
	prev := make([]int, len(nodes))
	done := make([]bool, len(nodes))
	for i := range cost {
		cost[i], prev[i] = inf, -1
	}
	cost[0] = 0
	for {
		best := -1
		for i := range nodes {
			if !done[i] && cost[i] < inf && (best < 0 || cost[i] < cost[best]) {
				best = i
			}
		}
		if best < 0 {
			break
		}
		done[best] = true
		for j := range nodes {
			if done[j] || !check.Works(nodes[best], nodes[j]) {
				continue
			}
			step := 0
			if !nodes[j].Existing && j != toIdx {
				step = 1
			}
			if cost[best]+step < cost[j] {
				cost[j], prev[j] = cost[best]+step, best
			}
		}
	}
	if cost[toIdx] >= inf {
		return Route{}, fmt.Errorf("planning: no route")
	}
	return buildRoute(nodes, prev, toIdx), nil
}

func buildRoute(nodes []Site, prev []int, toIdx int) Route {
	var chain []Site
	for i := toIdx; i >= 0; i = prev[i] {
		chain = append([]Site{nodes[i]}, chain...)
		if prev[i] < 0 {
			break
		}
	}
	r := Route{Sites: chain}
	for i, s := range chain {
		if !s.Existing && i != 0 && i != len(chain)-1 {
			r.NewSites++
		}
		if s.UncertaintyKm > 0 {
			r.UncertainSites++
			r.WorstUncertaintyKm = math.Max(r.WorstUncertaintyKm, s.UncertaintyKm)
		}
		if i > 0 {
			if d := distanceKm(chain[i-1], s); d > r.LongestHopKm {
				r.LongestHopKm = d
			}
		}
	}
	return r
}

// highGround finds candidate sites: local maxima on a coarse grid across the
// corridor between the two ends.
//
// Hills, not a uniform grid. A repeater goes on high ground, and offering a
// planner a thousand points in a valley is offering nothing — which the demo
// network proved the hard way when every path between three villages failed.
func highGround(from, to Site, t Terrain, o BridgeOptions) ([]Site, error) {
	if t == nil {
		return nil, nil
	}
	south, north := math.Min(from.Lat, to.Lat), math.Max(from.Lat, to.Lat)
	west, east := math.Min(from.Lon, to.Lon), math.Max(from.Lon, to.Lon)
	// A corridor rather than the straight line: the best relay is frequently
	// off to one side, on a hill the direct path misses.
	pad := math.Max(0.05, (north-south)*0.3)
	south, north = south-pad, north+pad
	west, east = west-pad, east+pad

	if err := checkScanFits(south, north, west, east, o.CandidateStep, "candidate"); err != nil {
		return nil, err
	}

	type cell struct {
		lat, lon, h float64
	}
	var cells []cell
	for lat := south; lat <= north; lat += o.CandidateStep {
		for lon := west; lon <= east; lon += o.CandidateStep {
			h, ok := t.ElevationM(lat, lon)
			if !ok {
				continue
			}
			cells = append(cells, cell{lat, lon, h})
		}
	}
	if len(cells) == 0 {
		return nil, fmt.Errorf("planning: no terrain covers the area between these points")
	}
	sort.Slice(cells, func(i, j int) bool { return cells[i].h > cells[j].h })

	// Highest first, thinned so two candidates are never the same hill.
	const minSeparationKm = 3.0
	var out []Site
	for _, c := range cells {
		s := Site{Lat: c.lat, Lon: c.lon, GroundM: c.h, HeightAGLm: o.MastHeightM}
		tooClose := false
		for _, p := range out {
			if distanceKm(p, s) < minSeparationKm {
				tooClose = true
				break
			}
		}
		if !tooClose {
			s.Name = fmt.Sprintf("proposed-%d", len(out)+1)
			out = append(out, s)
		}
		if len(out) >= 40 {
			break
		}
	}
	return out, nil
}

func sameSite(a, b Site) bool {
	return math.Abs(a.Lat-b.Lat) < 1e-9 && math.Abs(a.Lon-b.Lon) < 1e-9
}

func duplicateRoute(routes []Route, r Route) bool {
	for _, existing := range routes {
		if len(existing.Sites) != len(r.Sites) {
			continue
		}
		same := true
		for i := range r.Sites {
			if !sameSite(existing.Sites[i], r.Sites[i]) {
				same = false
				break
			}
		}
		if same {
			return true
		}
	}
	return false
}

// distanceKm is geo.DistanceKm over this package's own site type.
func distanceKm(a, b Site) float64 {
	return geo.DistanceKm(a.Lat, a.Lon, b.Lat, b.Lon)
}
