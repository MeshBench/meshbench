package planning_test

import (
	"testing"

	"github.com/A13xB0/meshcoresim/internal/coverage"
	"github.com/A13xB0/meshcoresim/internal/planning"
)

// Rasters built by hand, so the tests assert against a network whose shape is
// stated rather than one that fell out of a propagation model.
func raster(w, h int, workable func(x, y int) bool) *coverage.Raster {
	r := &coverage.Raster{Width: w, Height: h, South: 56, North: 57, West: -4, East: -3}
	r.Cells = make([]coverage.Cell, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if workable(x, y) {
				r.Cells[y*w+x] = coverage.Cell{OutboundMarginDB: 10, InboundMarginDB: 5}
			} else {
				r.Cells[y*w+x] = coverage.Cell{OutboundMarginDB: -20, InboundMarginDB: -30}
			}
		}
	}
	return r
}

// The site that adds most is not the site that covers most. A candidate whose
// coverage lands entirely on top of an existing repeater adds nothing, however
// large it looks on its own.
func TestBestSiteIsNotTheBiggestSite(t *testing.T) {
	const w, h = 20, 10
	existing, err := coverage.Combine([]*coverage.Raster{
		raster(w, h, func(x, _ int) bool { return x < 10 }),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Huge, but entirely inside what is already served.
	overlapping := raster(w, h, func(x, _ int) bool { return x < 10 })
	// Small, and squarely in the gap.
	filling := raster(w, h, func(x, _ int) bool { return x >= 14 && x < 17 })

	scores, err := planning.RankSites(existing,
		[]*coverage.Raster{overlapping, filling},
		[]planning.Candidate{{Name: "overlapping"}, {Name: "gap-filler"}})
	if err != nil {
		t.Fatal(err)
	}

	if scores[0].Candidate.Name != "gap-filler" {
		t.Errorf("ranked %q first; the gap-filler adds %d new cells against %d",
			scores[0].Candidate.Name, scores[1].NewCellsServed, scores[0].NewCellsServed)
	}
	if scores[0].NewCellsServed != 30 {
		t.Errorf("gap-filler added %d new cells, want 30", scores[0].NewCellsServed)
	}
	// Its own coverage is much smaller than the loser's, which is the point.
	if scores[0].OwnCellsServed >= scores[1].OwnCellsServed {
		t.Errorf("the winning site covers %d cells and the loser %d; this test is not testing what it claims",
			scores[0].OwnCellsServed, scores[1].OwnCellsServed)
	}
}

// Resilience is separate from reach. A network with no gaps can still be one
// mast away from failing, so a candidate that seconds an existing single server
// has to be visible in the score.
func TestRedundancyIsCountedSeparately(t *testing.T) {
	const w, h = 20, 10
	existing, err := coverage.Combine([]*coverage.Raster{
		raster(w, h, func(_, _ int) bool { return true }),
	})
	if err != nil {
		t.Fatal(err)
	}
	backup := raster(w, h, func(x, _ int) bool { return x < 5 })

	scores, err := planning.RankSites(existing,
		[]*coverage.Raster{backup}, []planning.Candidate{{Name: "backup"}})
	if err != nil {
		t.Fatal(err)
	}
	if scores[0].NewCellsServed != 0 {
		t.Errorf("a site inside full coverage claimed %d new cells", scores[0].NewCellsServed)
	}
	if scores[0].RedundancyAdded != 50 {
		t.Errorf("backed up %d single-served cells, want 50", scores[0].RedundancyAdded)
	}
}

func TestRankSitesRefusesMismatchedGrids(t *testing.T) {
	existing, err := coverage.Combine([]*coverage.Raster{raster(20, 10, func(int, int) bool { return true })})
	if err != nil {
		t.Fatal(err)
	}
	_, err = planning.RankSites(existing,
		[]*coverage.Raster{raster(5, 5, func(int, int) bool { return true })},
		[]planning.Candidate{{Name: "wrong-grid"}})
	if err == nil {
		t.Fatal("a candidate over a different grid was scored")
	}
}

// The useful output of a height sweep is the differences, not the totals — and
// the knee is where the money stops buying coverage.
func TestHeightSweepFindsTheKnee(t *testing.T) {
	heights := []float64{6, 12, 20, 30}
	// Coverage that saturates: 6 m is poor, 12 m is most of it, and beyond that
	// there is almost nothing left to gain.
	widths := []int{4, 14, 16, 17}
	var rasters []*coverage.Raster
	for _, wdt := range widths {
		w := wdt
		rasters = append(rasters, raster(20, 10, func(x, _ int) bool { return x < w }))
	}

	sweep, err := planning.HeightSweep(heights, rasters)
	if err != nil {
		t.Fatal(err)
	}
	if sweep[0].AddedOverPrevious != 0 {
		t.Error("the first step in a sweep has nothing to be measured against")
	}
	if sweep[1].AddedOverPrevious != 100 {
		t.Errorf("6 m to 12 m added %d cells, want 100", sweep[1].AddedOverPrevious)
	}

	knee, ok := planning.KneeHeight(sweep, 0.25)
	if !ok {
		t.Fatal("no knee found")
	}
	if knee != 12 {
		t.Errorf("knee at %.0f m, want 12 m — beyond that each step adds a fifth as much", knee)
	}
}

// Sweeps arrive in whatever order the caller computed them. Sorting by height
// is not cosmetic: differences taken in the wrong order are meaningless.
func TestHeightSweepSortsByHeight(t *testing.T) {
	heights := []float64{30, 6, 12}
	widths := []int{17, 4, 14}
	var rasters []*coverage.Raster
	for _, wdt := range widths {
		w := wdt
		rasters = append(rasters, raster(20, 10, func(x, _ int) bool { return x < w }))
	}
	sweep, err := planning.HeightSweep(heights, rasters)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(sweep); i++ {
		if sweep[i].HeightAGLm <= sweep[i-1].HeightAGLm {
			t.Fatalf("sweep is not in height order: %v", sweep)
		}
		if sweep[i].AddedOverPrevious < 0 {
			t.Errorf("height %.0f m lost coverage, which means the differences are misaligned",
				sweep[i].HeightAGLm)
		}
	}
}

// "Add 3 dB" is useless until you know at which end. The commonest real answer
// is that the handheld cannot be heard, and no work on the repeater's
// transmitter changes that.
func TestSummariseNamesTheLimitingDirection(t *testing.T) {
	inboundLimited := planning.Summarise("GB7XYZ", "handheld", 12,
		coverage.Cell{OutboundMarginDB: 14, InboundMarginDB: 1})
	if inboundLimited.LimitedBy != "inbound" {
		t.Errorf("limited by %q, want inbound", inboundLimited.LimitedBy)
	}
	if inboundLimited.WorstCaseDB != 1 {
		t.Errorf("worst case %.1f dB, want 1", inboundLimited.WorstCaseDB)
	}

	balanced := planning.Summarise("a", "b", 5,
		coverage.Cell{OutboundMarginDB: 6, InboundMarginDB: 6.4})
	if balanced.LimitedBy != "balanced" {
		t.Errorf("limited by %q, want balanced", balanced.LimitedBy)
	}

	oneWay := planning.Summarise("a", "b", 30,
		coverage.Cell{OutboundMarginDB: 4, InboundMarginDB: -9})
	if !oneWay.OneWayOnly || oneWay.Workable {
		t.Error("a link that closes one way only was reported as workable")
	}
}
