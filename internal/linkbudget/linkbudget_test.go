package linkbudget

import (
	"math"
	"testing"

	"github.com/A13xB0/meshcoresim/internal/antenna"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

func node(txDBm, gainDBi, feedDB float64, sf int) scenario.Node {
	n := scenario.Node{TxPowerDBm: txDBm}
	n.Radio.SpreadFactor = sf
	n.Radio.BandwidthHz = 250e3
	n.Antenna.FeedlineDB = feedDB
	n.Antenna.Pattern = &antenna.Collinear{GainDBiPeak: gainDBi}
	return n
}

// The property the whole package exists for: the margin is the weaker
// direction, so a mast that can be heard by a handheld it cannot hear back
// does not report a working link.
func TestMarginIsTheWeakerDirection(t *testing.T) {
	mast := node(22, 6, 0.8, 10)
	handheld := node(14, 0, 0, 10)
	const loss = 130

	out := OneWayDB(mast, handheld, loss)
	back := OneWayDB(handheld, mast, loss)
	if out <= back {
		t.Fatalf("setup: expected the mast to be the stronger direction, got %.1f and %.1f",
			out, back)
	}
	if got := MarginDB(mast, handheld, loss); got != back {
		t.Fatalf("margin %.2f, want the weaker direction %.2f", got, back)
	}
}

// And it is symmetric: which node is named first cannot change the answer.
func TestMarginIsSymmetric(t *testing.T) {
	a, b := node(22, 6, 0.8, 10), node(14, 2.15, 0.2, 10)
	if x, y := MarginDB(a, b, 128), MarginDB(b, a, 128); x != y {
		t.Fatalf("%.3f one way round, %.3f the other", x, y)
	}
}

// A node with no antenna still has a feedline, and losing it silently would
// flatter every link by that much.
func TestAMissingPatternStillPaysTheFeedline(t *testing.T) {
	n := scenario.Node{}
	n.Antenna.FeedlineDB = 1.5
	if got := GainDBi(n); got != -1.5 {
		t.Fatalf("gain %.2f, want -1.5", got)
	}
}

// Every spreading factor the radio supports has a threshold, and a higher one
// must be harder to decode than a lower one.
func TestSpreadingFactorsAreOrdered(t *testing.T) {
	prev := math.Inf(1)
	for sf := 7; sf <= 12; sf++ {
		n := scenario.Node{}
		n.Radio.SpreadFactor = sf
		got := RequiredSNRDB(n)
		if got >= prev {
			t.Fatalf("SF%d needs %.1f dB, which is not below SF%d's %.1f",
				sf, got, sf-1, prev)
		}
		prev = got
	}
}

// An unknown spreading factor gets a stated default rather than zero, which
// would claim a link decodes at the noise floor.
func TestAnUnknownSpreadingFactorHasAStatedDefault(t *testing.T) {
	n := scenario.Node{}
	n.Radio.SpreadFactor = 0
	if got := RequiredSNRDB(n); got != -15 {
		t.Fatalf("got %.1f, want the SF10 default of -15", got)
	}
}

// The breakdown must add up to the number it is breaking down. A budget panel
// whose lines do not sum to the margin beside it invites somebody to trust the
// wrong one.
func TestTermsSumToTheOneWayMargin(t *testing.T) {
	a, b := node(22, 6, 0.8, 10), node(14, 2.15, 0.2, 10)
	const loss = 131.5

	var sum float64
	for _, term := range Terms(a, b, loss) {
		sum += term.DB
	}
	if got := OneWayDB(a, b, loss); math.Abs(sum-got) > 1e-9 {
		t.Fatalf("terms sum to %.6f, one-way margin is %.6f", sum, got)
	}
}
