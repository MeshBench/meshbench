package planning_test

import (
	"math"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/planning"
)

// rangeChecker: a link works within a fixed distance. Stated geometry rather
// than a propagation model, so a routing bug cannot hide behind terrain.
type rangeChecker struct{ km float64 }

func (r rangeChecker) Works(a, b planning.Site) bool {
	return distKm(a, b) <= r.km
}

func distKm(a, b planning.Site) float64 {
	dLat := (b.Lat - a.Lat) * 111.32
	dLon := (b.Lon - a.Lon) * 61.3
	return sqrt(dLat*dLat + dLon*dLon)
}

func sqrt(v float64) float64 {
	if v <= 0 {
		return 0
	}
	x := v
	for i := 0; i < 40; i++ {
		x = (x + v/x) / 2
	}
	return x
}

// hills is terrain with summits about every 10 km along the corridor.
//
// Not flat ground. The candidate generator looks for local maxima, so flat
// terrain gives it nothing to rank and it returns an arbitrary scatter of
// points — which tests the sort, not the search. Real repeaters go on hills, and
// a test whose terrain has none is asking a question nobody has.
type hills struct{}

func (hills) ElevationM(lat, lon float64) (float64, bool) {
	// ~0.09 degrees of latitude is 10 km. The longitude term is smaller so the
	// ridge runs north-south, which is the direction the tests bridge across.
	return 300 + 200*math.Cos(lat/0.09*2*math.Pi) + 40*math.Cos(lon/0.05*2*math.Pi), true
}

// The point of the search: existing infrastructure is free, so a longer route
// over sites that already exist beats a shorter one needing new masts. A plain
// shortest-path search returns the opposite.
func TestExistingSitesAreReusedForFree(t *testing.T) {
	from := planning.Site{Lat: 56.40, Lon: -3.90, HeightAGLm: 10, Name: "A"}
	to := planning.Site{Lat: 56.76, Lon: -3.90, HeightAGLm: 10, Name: "B"}

	// A chain of existing repeaters covering the whole gap in 12 km steps.
	var existing []planning.Site
	for lat := 56.49; lat < 56.76; lat += 0.09 {
		existing = append(existing, planning.Site{
			Lat: lat, Lon: -3.90, HeightAGLm: 10, Existing: true,
		})
	}

	routes, err := planning.Bridge(from, to, hills{}, rangeChecker{km: 12}, planning.BridgeOptions{
		Existing: existing, MaxNew: 4, CandidateStep: 0.03, Alternatives: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if routes[0].NewSites != 0 {
		t.Errorf("a route existed entirely over built sites, but the search proposed %d new ones",
			routes[0].NewSites)
	}
	if len(routes[0].Sites) < 3 {
		t.Errorf("route has %d sites; it should hop through the existing chain", len(routes[0].Sites))
	}
}

// Where there is no existing infrastructure it has to propose some, and say how
// many.
func TestBridgeProposesNewSitesWhenItMust(t *testing.T) {
	from := planning.Site{Lat: 56.40, Lon: -3.90, HeightAGLm: 10}
	to := planning.Site{Lat: 56.76, Lon: -3.90, HeightAGLm: 10}

	routes, err := planning.Bridge(from, to, hills{}, rangeChecker{km: 12}, planning.BridgeOptions{
		MaxNew: 5, CandidateStep: 0.03,
	})
	if err != nil {
		t.Fatal(err)
	}
	if routes[0].NewSites == 0 {
		t.Error("a 40 km gap with 12 km links needs new sites")
	}
	if routes[0].LongestHopKm > 12.001 {
		t.Errorf("longest hop %.1f km exceeds what the checker allows", routes[0].LongestHopKm)
	}
}

// A search that cannot answer must say so rather than returning a route that
// does not work.
func TestBridgeRefusesWhenTheGapIsTooBig(t *testing.T) {
	from := planning.Site{Lat: 56.0, Lon: -3.9, HeightAGLm: 10}
	to := planning.Site{Lat: 57.5, Lon: -3.9, HeightAGLm: 10}

	_, err := planning.Bridge(from, to, hills{}, rangeChecker{km: 5}, planning.BridgeOptions{
		MaxNew: 2, CandidateStep: 0.05,
	})
	if err == nil {
		t.Fatal("a 165 km gap was bridged with two new sites")
	}
	if !strings.Contains(err.Error(), "new sites") {
		t.Errorf("the error should say what the limit was: %v", err)
	}
}

// Coverage placements come out in build order with their marginal gain, because
// a planner who can only afford two of five needs the right two.
func TestCoverAreaRanksByWhatEachSiteAdds(t *testing.T) {
	area := []planning.LatLon{
		{Lat: 56.40, Lon: -3.95}, {Lat: 56.40, Lon: -3.55},
		{Lat: 56.70, Lon: -3.55}, {Lat: 56.70, Lon: -3.95},
	}
	placements, err := planning.CoverArea(area, hills{}, rangeChecker{km: 8}, planning.CoverOptions{
		MaxNew: 4, SampleStep: 0.03, CandidateStep: 0.05,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(placements) == 0 {
		t.Fatal("no sites proposed for an empty area")
	}
	for i := 1; i < len(placements); i++ {
		if placements[i].NewCellsCovered > placements[i-1].NewCellsCovered {
			t.Errorf("site %d adds more than site %d; they are not in build order", i, i-1)
		}
		if placements[i].CoverageAfterPct < placements[i-1].CoverageAfterPct {
			t.Error("coverage went down as sites were added")
		}
	}
	if placements[len(placements)-1].CoverageAfterPct <= 0 {
		t.Error("no coverage was achieved at all")
	}
}

// Proposing sites that cover nothing new, to fill a requested count, is how a
// planner ends up building one.
func TestCoverAreaStopsWhenNothingIsLeftToAdd(t *testing.T) {
	area := []planning.LatLon{
		{Lat: 56.50, Lon: -3.90}, {Lat: 56.50, Lon: -3.80},
		{Lat: 56.56, Lon: -3.80}, {Lat: 56.56, Lon: -3.90},
	}
	// A range far larger than the area: one site covers everything.
	placements, err := planning.CoverArea(area, hills{}, rangeChecker{km: 200}, planning.CoverOptions{
		MaxNew: 6, SampleStep: 0.02, CandidateStep: 0.03,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(placements) != 1 {
		t.Errorf("proposed %d sites where one covers the whole area", len(placements))
	}
}

// Existing infrastructure is credited first, or a site inside an already-served
// town outranks one filling a genuine gap.
func TestExistingCoverageIsNotCreditedToNewSites(t *testing.T) {
	area := []planning.LatLon{
		{Lat: 56.40, Lon: -3.95}, {Lat: 56.40, Lon: -3.55},
		{Lat: 56.70, Lon: -3.55}, {Lat: 56.70, Lon: -3.95},
	}
	opts := planning.CoverOptions{MaxNew: 2, SampleStep: 0.03, CandidateStep: 0.05}

	bare, err := planning.CoverArea(area, hills{}, rangeChecker{km: 8}, opts)
	if err != nil {
		t.Fatal(err)
	}

	opts.Existing = []planning.Site{{Lat: 56.55, Lon: -3.75, HeightAGLm: 10, Existing: true}}
	withOne, err := planning.CoverArea(area, hills{}, rangeChecker{km: 8}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if withOne[0].NewCellsCovered >= bare[0].NewCellsCovered {
		t.Errorf("a new site was credited with %d cells even though an existing one already "+
			"covered part of the area (bare: %d)", withOne[0].NewCellsCovered, bare[0].NewCellsCovered)
	}

	base, err := planning.BaselineCoverage(area, hills{}, rangeChecker{km: 8}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if base <= 0 {
		t.Error("an existing site covered nothing")
	}
}
