package coverage_test

import (
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/study/coverage"
)

// The same station, priced twice: once as a surveyed mast and once as a record
// somebody imported at +/-5 km. Identical geometry, and it must not read as an
// identically good answer.
func TestALooselyPlacedStationCoversLess(t *testing.T) {
	// A long radial rather than the usual box: near the station every link
	// closes by tens of decibels and a few of them change nothing, so the cells
	// that can move are the ones out at the edge of the reach, and a box that
	// does not contain that edge cannot show the difference.
	radial := func() *coverage.Raster {
		return &coverage.Raster{
			South: 56.7, North: 56.71, West: -3.9, East: -1.9,
			Width: 1200, Height: 1, FreqMHz: 869.525,
		}
	}
	surveyed, loose := radial(), radial()
	if err := coverage.Compute(station(), flat{100}, surveyed, opts()); err != nil {
		t.Fatal(err)
	}
	uncertain := station()
	uncertain.UncertaintyKm = 5
	if err := coverage.Compute(uncertain, flat{100}, loose, opts()); err != nil {
		t.Fatal(err)
	}

	var confidentCells, looseCells int
	for i := range surveyed.Cells {
		if surveyed.Cells[i].Workable() {
			confidentCells++
		}
		if loose.Cells[i].Workable() {
			looseCells++
		}
		// The best case is the same picture. Only the doubt is new.
		if surveyed.Cells[i].OutboundMarginDB != loose.Cells[i].OutboundMarginDB {
			t.Fatalf("cell %d: uncertainty moved the best-case margin", i)
		}
	}
	if confidentCells == 0 {
		t.Fatal("the surveyed station covered nothing; this test is not testing what it claims")
	}
	if looseCells >= confidentCells {
		t.Fatalf("a station imported at +/-5 km covered %d cells against the surveyed %d",
			looseCells, confidentCells)
	}
}

// Uncertainty is a property of the path, so it costs both directions the same
// number of decibels - and the two directions still answer separately, which is
// the only way a cell is allowed to answer.
func TestUncertaintyDoesNotCollapseTheDirections(t *testing.T) {
	uncertain := station()
	uncertain.UncertaintyKm = 4
	r := &coverage.Raster{
		South: 56.7, North: 56.71, West: -3.9, East: -2.9,
		Width: 400, Height: 1, FreqMHz: 869.525,
	}
	if err := coverage.Compute(uncertain, flat{100}, r, opts()); err != nil {
		t.Fatal(err)
	}

	oneWay := 0
	for x := 0; x < r.Width; x++ {
		c := r.At(x, 0)
		if c.NoData {
			continue
		}
		if c.PositionSlackDB <= 0 {
			t.Fatalf("cell %d carried no position slack", x)
		}
		best := c.OutboundMarginDB - c.InboundMarginDB
		worst := c.OutboundWorstCaseDB() - c.InboundWorstCaseDB()
		if math.Abs(best-worst) > 1e-9 {
			t.Fatalf("cell %d: the ends differ by %.3f dB before the band and %.3f dB after",
				x, best, worst)
		}
		if c.OneWay() {
			oneWay++
		}
	}
	if oneWay == 0 {
		t.Error("no cell where the station is heard but cannot hear; the asymmetry has been lost")
	}
}

// A cell that is only covered when the guess turns out to be right is not
// covered. Workable is the predicate everything downstream counts, so this is
// where the uncertainty has to bite.
func TestWorkableIsJudgedAtThePessimisticEnd(t *testing.T) {
	c := coverage.Cell{OutboundMarginDB: 6, InboundMarginDB: 3, PositionSlackDB: 8}
	if c.Workable() {
		t.Error("3 dB of margin with 8 dB of position doubt was called workable")
	}
	if !c.WorkableIfExact() {
		t.Error("the same cell does close if the positions are exactly right")
	}
	if got := c.WorstCaseDB(); got != -5 {
		t.Errorf("worst case %.1f dB, want -5", got)
	}
	if got := c.OutboundWorstCaseDB(); got != -2 {
		t.Errorf("outbound worst case %.1f dB, want -2", got)
	}
	// No data is still ignorance rather than a result, whatever the band says.
	if (coverage.Cell{NoData: true, OutboundMarginDB: 20, InboundMarginDB: 20}).Workable() {
		t.Error("a cell with no terrain reported a workable link")
	}
}

// Combining is where a per-node answer becomes a network's, and it is the
// easiest place to drop the doubt on the way through.
func TestCombineKeepsTheDoubt(t *testing.T) {
	uncertain := station()
	uncertain.UncertaintyKm = 5
	r := grid()
	if err := coverage.Compute(uncertain, flat{100}, r, opts()); err != nil {
		t.Fatal(err)
	}
	c, err := coverage.Combine([]*coverage.Raster{r})
	if err != nil {
		t.Fatal(err)
	}
	slacked := 0
	for i, cell := range c.Cells {
		if cell.NoData {
			continue
		}
		if cell.PositionSlackDB > 0 {
			slacked++
		}
		if math.Abs(c.BestMarginDB[i]-cell.WorstCaseDB()) > 1e-9 {
			t.Fatalf("cell %d: best margin %.3f, worst case of the winning link %.3f",
				i, c.BestMarginDB[i], cell.WorstCaseDB())
		}
	}
	if slacked == 0 {
		t.Fatal("combining a raster from a +/-5 km station produced cells with no doubt in them")
	}
}

// Between two stations offering the same signal, the one whose position is
// known wins the cell. Anything else lets an unsurveyed record decide where the
// network is thought to reach.
func TestTheBetterKnownStationServesTheCell(t *testing.T) {
	surveyed, loose := grid(), grid()
	if err := coverage.Compute(station(), flat{100}, surveyed, opts()); err != nil {
		t.Fatal(err)
	}
	uncertain := station()
	uncertain.UncertaintyKm = 5
	if err := coverage.Compute(uncertain, flat{100}, loose, opts()); err != nil {
		t.Fatal(err)
	}

	c, err := coverage.Combine([]*coverage.Raster{loose, surveyed})
	if err != nil {
		t.Fatal(err)
	}
	for i := range c.Cells {
		if c.Cells[i].NoData || c.BestNode[i] < 0 {
			continue
		}
		if c.BestNode[i] != 1 {
			t.Fatalf("cell %d is served by the station nobody has surveyed", i)
		}
	}
}
