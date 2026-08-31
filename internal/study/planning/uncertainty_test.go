package planning_test

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/study/coverage"
	"github.com/MeshBench/meshbench/internal/study/planning"
)

// A network whose own positions are guesses has gaps it does not know about,
// and a candidate that fills them is being credited for somebody's paperwork.
// The score has to say so, or a mast gets built to duplicate a repeater that
// was there all along.
func TestNewCellsBoughtOnlyByDoubtAreCounted(t *testing.T) {
	const w, h = 20, 10
	// The existing repeater reaches the left half, and only if it really is
	// where it was imported to be.
	shaky := &coverage.Raster{Width: w, Height: h, South: 56, North: 57, West: -4, East: -3}
	shaky.Cells = make([]coverage.Cell, w*h)
	for i := range shaky.Cells {
		if i%w < 10 {
			shaky.Cells[i] = coverage.Cell{
				OutboundMarginDB: 10, InboundMarginDB: 5, PositionSlackDB: 9}
		} else {
			shaky.Cells[i] = coverage.Cell{OutboundMarginDB: -20, InboundMarginDB: -30}
		}
	}
	existing, err := coverage.Combine([]*coverage.Raster{shaky})
	if err != nil {
		t.Fatal(err)
	}

	overlapping := raster(w, h, func(x, _ int) bool { return x < 10 })
	scores, err := planning.RankSites(existing,
		[]*coverage.Raster{overlapping}, []planning.Candidate{{Name: "overlapping"}})
	if err != nil {
		t.Fatal(err)
	}
	if scores[0].NewCellsServed != 100 {
		t.Fatalf("the shaky repeater still counted as serving: %d new cells, want 100",
			scores[0].NewCellsServed)
	}
	if scores[0].UncertainNewCells != 100 {
		t.Errorf("%d of the new cells were flagged as bought by doubt, want 100",
			scores[0].UncertainNewCells)
	}

	// The same score against a surveyed network is genuinely new ground.
	sure, err := coverage.Combine([]*coverage.Raster{
		raster(w, h, func(x, _ int) bool { return x >= 10 })})
	if err != nil {
		t.Fatal(err)
	}
	scores, err = planning.RankSites(sure,
		[]*coverage.Raster{overlapping}, []planning.Candidate{{Name: "overlapping"}})
	if err != nil {
		t.Fatal(err)
	}
	if scores[0].UncertainNewCells != 0 {
		t.Errorf("%d cells were blamed on doubt in a surveyed network", scores[0].UncertainNewCells)
	}
}

// Two sites that buy exactly the same thing are not equally good bets: only the
// one whose position is known is a site rather than an area.
func TestASurveyedCandidateWinsATie(t *testing.T) {
	const w, h = 20, 10
	existing, err := coverage.Combine([]*coverage.Raster{
		raster(w, h, func(x, _ int) bool { return x < 10 })})
	if err != nil {
		t.Fatal(err)
	}
	same := func() *coverage.Raster { return raster(w, h, func(x, _ int) bool { return x >= 14 }) }

	scores, err := planning.RankSites(existing,
		[]*coverage.Raster{same(), same()},
		[]planning.Candidate{{Name: "guessed", UncertaintyKm: 5}, {Name: "surveyed"}})
	if err != nil {
		t.Fatal(err)
	}
	if scores[0].NewCellsServed != scores[1].NewCellsServed {
		t.Fatal("the two candidates were not tied; this test is not testing what it claims")
	}
	if scores[0].Candidate.Name != "surveyed" {
		t.Errorf("ranked %q above the surveyed site on an exact tie", scores[0].Candidate.Name)
	}
}

// A margin summary is the sentence somebody acts on, so the doubt has to arrive
// with it rather than being left in the cell it came from.
func TestSummariseCarriesTheBand(t *testing.T) {
	l := planning.Summarise("GB7XYZ", "handheld", 12,
		coverage.Cell{OutboundMarginDB: 14, InboundMarginDB: 4, PositionSlackDB: 9})

	if l.PositionSlackDB != 9 {
		t.Errorf("slack %.1f dB, want 9", l.PositionSlackDB)
	}
	if l.WorstCaseDB != -5 {
		t.Errorf("worst case %.1f dB, want -5", l.WorstCaseDB)
	}
	if l.Workable {
		t.Error("a link that needs the far end to be exactly where it was imported was called workable")
	}
	if !l.OneWayOnly {
		t.Error("outbound still closes and inbound does not; that is the answer worth having")
	}
	// The best case is reported unchanged beside it, so the two numbers can be
	// read against each other.
	if l.InboundDB != 4 {
		t.Errorf("inbound %.1f dB, want the unmodified 4", l.InboundDB)
	}
}

// A route through a repeater that might be five kilometres away is a plan built
// on a guess, and the hop lengths in it are only as good as that position.
func TestRoutesReportTheirWorstPosition(t *testing.T) {
	from := planning.Site{Lat: 56.40, Lon: -3.90, HeightAGLm: 10, Name: "A"}
	to := planning.Site{Lat: 56.76, Lon: -3.90, HeightAGLm: 10, Name: "B"}
	existing := []planning.Site{
		{Lat: 56.52, Lon: -3.90, HeightAGLm: 10, Existing: true, Name: "surveyed"},
		{Lat: 56.64, Lon: -3.90, HeightAGLm: 10, Existing: true, Name: "imported",
			UncertaintyKm: 5},
	}

	routes, err := planning.Bridge(from, to, hills{}, rangeChecker{km: 15},
		planning.BridgeOptions{Existing: existing, MastHeightM: 10, MaxNew: 2,
			CandidateStep: 0.05, Alternatives: 1})
	if err != nil {
		t.Fatal(err)
	}
	best := routes[0]
	if best.UncertainSites != 1 {
		t.Errorf("%d sites in the route are unsurveyed, want 1: %v", best.UncertainSites, best.Sites)
	}
	if best.WorstUncertaintyKm != 5 {
		t.Errorf("worst position is good to %.1f km, want 5", best.WorstUncertaintyKm)
	}
}
