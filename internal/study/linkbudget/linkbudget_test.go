package linkbudget

import (
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

func node(txDBm, gainDBi, feedDB float64, sf int) scenario.Node {
	n := scenario.Node{TxPowerDBm: txDBm}
	n.Radio.SpreadFactor = sf
	n.Radio.BandwidthHz = 250e3
	n.Antenna.FeedlineDB = feedDB
	n.Antenna.Pattern = &antenna.Collinear{GainDBiPeak: gainDBi}
	return n
}

func at(n scenario.Node, lat, lon float64) scenario.Node {
	n.Position = scenario.LatLon{Lat: lat, Lon: lon}
	return n
}

// beam replaces a node's antenna with a yagi aimed at a compass bearing: 14
// dBi on the nose and 20 dB down off the back, which is an ordinary one.
func beam(n scenario.Node, bearingDeg float64) scenario.Node {
	n.Antenna.Pattern = antenna.Yagi{GainDBiPeak: 14, BeamwidthDeg: 40, FrontToBackDB: 20}
	n.Antenna.BearingDeg = bearingDeg
	return n
}

// The property the whole package exists for: the margin is the weaker
// direction, so a mast that can be heard by a handheld it cannot hear back
// does not report a working link.
func TestMarginIsTheWeakerDirection(t *testing.T) {
	mast := at(node(22, 6, 0.8, 10), 56.0, -4.0)
	handheld := at(node(14, 0, 0, 10), 56.0, -3.0)
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
	a := at(node(22, 6, 0.8, 10), 56.0, -4.0)
	b := at(node(14, 2.15, 0.2, 10), 56.0, -3.0)
	if x, y := MarginDB(a, b, 128), MarginDB(b, a, 128); x != y {
		t.Fatalf("%.3f one way round, %.3f the other", x, y)
	}
}

// A node with no antenna still has a feedline, and losing it silently would
// flatter every link by that much.
func TestAMissingPatternStillPaysTheFeedline(t *testing.T) {
	n := scenario.Node{}
	n.Antenna.FeedlineDB = 1.5
	if got := GainDBi(n, at(scenario.Node{}, 56.5, -4.0)); got != -1.5 {
		t.Fatalf("gain %.2f, want -1.5", got)
	}
}

// The rule the package is held to: a beam is worth what it is worth in the
// direction the far end is actually in. Pricing the peak instead is not a
// rough answer, it is a wrong one in the flattering direction, and an operator
// would build the link on it.
func TestABeamPointedAwayIsChargedForIt(t *testing.T) {
	const loss = 130
	// Two stations a degree of longitude apart at 56 N: b lies due east of a.
	handheld := at(node(14, 2.15, 0, 10), 56.0, -3.0)
	towards := beam(at(node(22, 0, 0.8, 10), 56.0, -4.0), 90)
	away := beam(at(node(22, 0, 0.8, 10), 56.0, -4.0), 270)

	gain, back := GainDBi(towards, handheld), GainDBi(away, handheld)
	if gain-back < 15 {
		t.Fatalf("a yagi on the nose gave %.1f dBi and the same one backwards %.1f dBi; "+
			"pointing is not being evaluated", gain, back)
	}
	if closed, lost := MarginDB(towards, handheld, loss), MarginDB(away, handheld, loss); closed-lost < 15 {
		t.Errorf("margin %.1f dB pointed at the far end against %.1f dB pointed away: "+
			"the budget is being priced off peak gain", closed, lost)
	}
}

// Reachability is asymmetric, and so is pointing. Two beams both aimed east
// mean one of them is looking at the far end and the other is looking away, so
// the gain credited to each direction has to be worked out separately.
func TestEachDirectionGetsItsOwnBearing(t *testing.T) {
	a := beam(at(node(22, 0, 0, 10), 56.0, -4.0), 90) // east, at b
	b := beam(at(node(22, 0, 0, 10), 56.0, -3.0), 90) // east, away from a

	toB, toA := GainDBi(a, b), GainDBi(b, a)
	if toB-toA < 15 {
		t.Fatalf("a sees b at %.1f dBi and b sees a at %.1f dBi; both directions "+
			"are being given the same answer", toB, toA)
	}

	// And the breakdown has to name them the same way round, or a panel shows
	// the receiving antenna's gain against the transmitting antenna's line.
	out, home := Terms(a, b, 130), Terms(b, a, 130)
	if out[1].DB != toB || out[3].DB != toA {
		t.Errorf("a to b breaks down as tx %.1f, rx %.1f; want %.1f and %.1f",
			out[1].DB, out[3].DB, toB, toA)
	}
	if home[1].DB != toA || home[3].DB != toB {
		t.Errorf("b to a breaks down as tx %.1f, rx %.1f; want %.1f and %.1f",
			home[1].DB, home[3].DB, toA, toB)
	}
}

// The other half of the rule: elevation is read at the boresight, because this
// package is handed no ground to work an elevation angle out of. A mast tilted
// at a valley it cannot see must not be charged for the tilt here.
func TestElevationIsReadAtTheBoresight(t *testing.T) {
	mast := at(node(22, 8, 0.5, 10), 56.0, -4.0)
	mast.Antenna.DowntiltDeg = 10
	far := at(node(14, 2.15, 0, 10), 56.0, -3.0)

	// A collinear that steep in the elevation plane is more than 10 dB down at
	// ten degrees off, so a tilt charged here would be impossible to miss.
	if got, want := GainDBi(mast, far), 8-0.5; math.Abs(got-want) > 1e-9 {
		t.Fatalf("gain %.2f dBi, want the boresight figure %.2f", got, want)
	}
}

// A planned site is checked against itself, and two nodes at one point have no
// direction between them. The best case is the only answer that is not an
// arbitrary bearing.
func TestNodesAtOnePointGetTheBestCase(t *testing.T) {
	n := beam(at(node(22, 0, 1.2, 10), 56.0, -4.0), 270)
	if got, want := GainDBi(n, n), 14-1.2; math.Abs(got-want) > 1e-9 {
		t.Fatalf("gain %.2f dBi, want the peak %.2f", got, want)
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
	a := at(node(22, 6, 0.8, 10), 56.0, -4.0)
	b := at(node(14, 2.15, 0.2, 10), 56.0, -3.0)
	const loss = 131.5

	var sum float64
	for _, term := range Terms(a, b, loss) {
		sum += term.DB
	}
	if got := OneWayDB(a, b, loss); math.Abs(sum-got) > 1e-9 {
		t.Fatalf("terms sum to %.6f, one-way margin is %.6f", sum, got)
	}
}
