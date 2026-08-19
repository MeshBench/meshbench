package planning

import (
	"fmt"
	"math"
	"sort"
)

// CoverOptions control an area-coverage search.
type CoverOptions struct {
	// Existing sites already serve part of the area, and only what they do not
	// reach is worth spending a new site on.
	Existing []Site

	MastHeightM float64

	// MaxNew is how many sites to place. The answer is a list in the order they
	// should be built, because the first one usually buys most of the coverage
	// and the last one frequently is not worth building at all.
	MaxNew int

	// SampleStep is the grid spacing in degrees for measuring coverage.
	SampleStep float64
	// CandidateStep is the spacing for places to consider putting a site.
	CandidateStep float64
}

// Placement is one proposed site and what it bought.
type Placement struct {
	Site Site
	// NewCellsCovered is how many previously-unserved sample points this site
	// reaches. The marginal figure, not the total, because that is what decides
	// whether it is worth building.
	NewCellsCovered int
	// CoverageAfterPct is the area covered once this site exists.
	CoverageAfterPct float64
}

// CoverArea places sites to cover as much of a polygon as possible.
//
// Greedy: place the site that adds most, then the next, and so on. Greedy is
// not optimal for maximum coverage — that problem is NP-hard — but it is within
// a known bound of optimal, and it has a property the optimal answer does not:
// the sites come out in build order, each one's marginal gain attached. A
// planner who can only afford two of the five gets the right two.
func CoverArea(area []LatLon, t Terrain, check LinkChecker, o CoverOptions) ([]Placement, error) {
	if len(area) < 3 {
		return nil, fmt.Errorf("planning: an area needs at least three corners")
	}
	if o.MaxNew <= 0 {
		o.MaxNew = 3
	}
	if o.MastHeightM <= 0 {
		o.MastHeightM = 10
	}
	if o.SampleStep <= 0 {
		o.SampleStep = 0.01
	}
	if o.CandidateStep <= 0 {
		o.CandidateStep = 0.02
	}

	samples, err := sampleArea(area, t, o)
	if err != nil {
		return nil, err
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("planning: no terrain covers this area")
	}

	candidates, err := coverCandidates(area, t, o)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("planning: no candidate sites inside this area")
	}

	covered := make([]bool, len(samples))
	// Existing infrastructure first, so a new site is only credited with what
	// it adds. Crediting it with everything it reaches would rank a site inside
	// an already-served town above one filling a genuine gap.
	for _, e := range o.Existing {
		for i, s := range samples {
			if !covered[i] && check.Works(e, s) {
				covered[i] = true
			}
		}
	}

	var out []Placement
	for len(out) < o.MaxNew {
		bestIdx, bestGain := -1, 0
		for ci, c := range candidates {
			gain := 0
			for i, s := range samples {
				if !covered[i] && check.Works(c, s) {
					gain++
				}
			}
			if gain > bestGain {
				bestIdx, bestGain = ci, gain
			}
		}
		if bestIdx < 0 || bestGain == 0 {
			// Nothing left to add. Stopping is the honest answer: proposing
			// sites that cover nothing new to fill a requested count is how a
			// planner ends up building one.
			break
		}

		site := candidates[bestIdx]
		for i, s := range samples {
			if !covered[i] && check.Works(site, s) {
				covered[i] = true
			}
		}
		candidates = append(candidates[:bestIdx], candidates[bestIdx+1:]...)

		done := 0
		for _, c := range covered {
			if c {
				done++
			}
		}
		site.Name = fmt.Sprintf("proposed-%d", len(out)+1)
		out = append(out, Placement{
			Site:             site,
			NewCellsCovered:  bestGain,
			CoverageAfterPct: 100 * float64(done) / float64(len(samples)),
		})
	}
	return out, nil
}

// BaselineCoverage is what the existing network already covers, so a proposal
// can be stated as a change rather than as a number on its own.
func BaselineCoverage(area []LatLon, t Terrain, check LinkChecker, o CoverOptions) (float64, error) {
	samples, err := sampleArea(area, t, o)
	if err != nil {
		return 0, err
	}
	if len(samples) == 0 {
		return 0, fmt.Errorf("planning: no terrain covers this area")
	}
	done := 0
	for _, s := range samples {
		for _, e := range o.Existing {
			if check.Works(e, s) {
				done++
				break
			}
		}
	}
	return 100 * float64(done) / float64(len(samples)), nil
}

// LatLon is a corner of an area.
type LatLon struct{ Lat, Lon float64 }

// sampleArea builds the points coverage is measured at.
//
// At ground level plus a receiver height, because coverage means "could
// somebody standing here be heard", not "could a mast here be heard".
func sampleArea(area []LatLon, t Terrain, o CoverOptions) ([]Site, error) {
	south, north, west, east := bounds(area)
	var out []Site
	for lat := south; lat <= north; lat += o.SampleStep {
		for lon := west; lon <= east; lon += o.SampleStep {
			if !inside(area, lat, lon) {
				continue
			}
			h, ok := t.ElevationM(lat, lon)
			if !ok {
				continue
			}
			// 1.5 m: a person with a handheld, which is who coverage is for.
			out = append(out, Site{Lat: lat, Lon: lon, GroundM: h, HeightAGLm: 1.5})
		}
	}
	return out, nil
}

// coverCandidates is the high ground inside the area.
func coverCandidates(area []LatLon, t Terrain, o CoverOptions) ([]Site, error) {
	south, north, west, east := bounds(area)
	type cell struct{ lat, lon, h float64 }
	var cells []cell
	for lat := south; lat <= north; lat += o.CandidateStep {
		for lon := west; lon <= east; lon += o.CandidateStep {
			if !inside(area, lat, lon) {
				continue
			}
			h, ok := t.ElevationM(lat, lon)
			if !ok {
				continue
			}
			cells = append(cells, cell{lat, lon, h})
		}
	}
	sort.Slice(cells, func(i, j int) bool { return cells[i].h > cells[j].h })

	var out []Site
	for _, c := range cells {
		s := Site{Lat: c.lat, Lon: c.lon, GroundM: c.h, HeightAGLm: o.MastHeightM}
		near := false
		for _, p := range out {
			if distanceKm(p, s) < 2 {
				near = true
				break
			}
		}
		if !near {
			out = append(out, s)
		}
		if len(out) >= 60 {
			break
		}
	}
	return out, nil
}

func bounds(area []LatLon) (south, north, west, east float64) {
	south, west = math.Inf(1), math.Inf(1)
	north, east = math.Inf(-1), math.Inf(-1)
	for _, p := range area {
		south = math.Min(south, p.Lat)
		north = math.Max(north, p.Lat)
		west = math.Min(west, p.Lon)
		east = math.Max(east, p.Lon)
	}
	return south, north, west, east
}

// inside is the even-odd ray test.
func inside(area []LatLon, lat, lon float64) bool {
	in := false
	for i, j := 0, len(area)-1; i < len(area); j, i = i, i+1 {
		if (area[i].Lat > lat) == (area[j].Lat > lat) {
			continue
		}
		x := (area[j].Lon-area[i].Lon)*(lat-area[i].Lat)/(area[j].Lat-area[i].Lat) + area[i].Lon
		if lon < x {
			in = !in
		}
	}
	return in
}
