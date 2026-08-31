package linkbudget

import (
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/world/scenario"
)

func placed(n scenario.Node, lat, lon, uncertaintyKm float64) scenario.Node {
	n.Position = scenario.LatLon{Lat: lat, Lon: lon}
	n.UncertaintyKm = uncertaintyKm
	return n
}

// The rule the whole thing exists for: a node nobody surveyed does not get the
// same answer as one somebody did.
func TestALoosePositionCostsMargin(t *testing.T) {
	mast := placed(node(22, 6, 0.8, 10), 56.7, -3.9, 0)
	surveyed := placed(node(14, 0, 0, 10), 56.79, -3.9, 0)
	// The same handheld, in the same place, imported at +/-5 km instead.
	guessed := placed(node(14, 0, 0, 10), 56.79, -3.9, 5)
	const loss = 120

	confident := OneWay(mast, surveyed, loss)
	loose := OneWay(mast, guessed, loss)

	if confident.PositionSlackDB != 0 {
		t.Fatalf("two surveyed positions carried %.2f dB of slack", confident.PositionSlackDB)
	}
	if loose.DB != confident.DB {
		t.Fatalf("the best case moved: %.3f dB against %.3f dB", loose.DB, confident.DB)
	}
	if loose.PositionSlackDB <= 0 {
		t.Fatal("a node imported at +/-5 km got a confident answer")
	}
	if loose.WorstCaseDB() >= confident.WorstCaseDB() {
		t.Fatalf("the uncertain link is worth %.2f dB against the surveyed %.2f dB; "+
			"uncertainty must widen the pessimistic side, never the other one",
			loose.WorstCaseDB(), confident.WorstCaseDB())
	}
}

// Both directions of one pair carry their own band. Collapsing them into a
// single "the link is uncertain" figure loses which end fails, which is the one
// thing an asymmetric answer is for.
func TestBothDirectionsKeepTheirOwnBand(t *testing.T) {
	mast := placed(node(22, 6, 0.8, 10), 56.7, -3.9, 3)
	handheld := placed(node(14, 0, 0, 10), 56.9, -3.9, 1)
	const loss = 130

	out := OneWay(mast, handheld, loss)
	back := OneWay(handheld, mast, loss)

	if out.PositionSlackDB <= 0 || back.PositionSlackDB <= 0 {
		t.Fatal("a pair of loosely placed nodes produced a link with no doubt in it")
	}
	// The asymmetry between the ends survives the widening: uncertainty is a
	// property of the path, so it must move both directions by the same amount
	// and leave the 13 dB between a 22 dBm mast and a 14 dBm handheld intact.
	best := out.DB - back.DB
	worst := out.WorstCaseDB() - back.WorstCaseDB()
	if math.Abs(best-worst) > 1e-9 {
		t.Fatalf("the directions differ by %.3f dB before the band and %.3f dB after; "+
			"the uncertainty has eaten the asymmetry", best, worst)
	}
	if out.WorstCaseDB() == back.WorstCaseDB() {
		t.Fatal("the two directions came out identical, so this test is not testing what it claims")
	}
}

// Uncertainty is only ever spent, never earned. A node that might be five
// kilometres nearer is not an argument for a better answer.
func TestSlackOnlyEverSubtracts(t *testing.T) {
	if got := PositionSlackDB(10, 0, 0); got != 0 {
		t.Fatalf("two surveyed ends cost %.3f dB", got)
	}
	if got := PositionSlackDB(10, 2, 3); got <= 0 {
		t.Fatalf("5 km of doubt over a 10 km path cost %.3f dB", got)
	}
	// The two radii add, priced at free space over the widened separation.
	want := 20 * math.Log10(15.0/10.0)
	if got := PositionSlackDB(10, 2, 3); math.Abs(got-want) > 1e-9 {
		t.Fatalf("slack %.4f dB, want %.4f dB", got, want)
	}
	// A short path is where a loose position hurts most, which is the right way
	// round: 5 km of doubt says nothing about a 200 km link and everything
	// about a 2 km one.
	if PositionSlackDB(2, 5, 0) <= PositionSlackDB(200, 5, 0) {
		t.Fatal("a loose position cost a long path more than a short one")
	}
	// A zero-length path has no defined free-space loss to widen, and must not
	// return an infinity that would poison every margin downstream.
	if got := PositionSlackDB(0, 5, 5); got != 0 {
		t.Fatalf("a zero-length path carried %.3f dB of slack", got)
	}
}

// Closes is decided at the pessimistic end. A margin that only closes when a
// guess turns out to be right has not been shown to close.
func TestClosesIsJudgedAtTheWorstCase(t *testing.T) {
	m := Margin{DB: 4, PositionSlackDB: 9}
	if m.Closes() {
		t.Fatal("a 4 dB margin with 9 dB of position doubt was called workable")
	}
	if got := m.WorstCaseDB(); got != -5 {
		t.Fatalf("worst case %.1f dB, want -5", got)
	}
}
