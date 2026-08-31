package linkbudget

import (
	"math"

	"github.com/MeshBench/meshbench/internal/rf/geo"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Margin is one direction's link margin together with the width that the two
// ends' position uncertainty gives it.
//
// A pair of numbers rather than one, because a margin computed from a surveyed
// mast and a margin computed from a node someone placed off a map are not the
// same kind of answer, and a bare decibel figure has nowhere to say which it
// is. Anyone who wants the number has to walk past the band to reach it.
type Margin struct {
	// DB is the margin at the receiving end; positive closes. It is the
	// optimistic end of the band: both ends exactly where they were reported.
	DB float64

	// PositionSlackDB is how much of DB the ends' position uncertainty could
	// take away. Never negative, and never added back on: uncertainty widens
	// the pessimistic side only.
	PositionSlackDB float64
}

// WorstCaseDB is the margin with the positions as wrong as they are allowed to
// be. It is the figure to decide on, because every other number here is
// already a best case and this is the only one that is not made better by
// nobody having surveyed the site.
func (m Margin) WorstCaseDB() float64 { return m.DB - m.PositionSlackDB }

// Closes reports whether the link still closes at that pessimistic end.
func (m Margin) Closes() bool { return m.WorstCaseDB() >= 0 }

// PositionSlackDB is how many decibels of margin the endpoints' position
// uncertainty could take away over a path of distKm, given the radius each end
// is placed to.
//
// Taken away only. A node reported to +/-5 km could equally be 5 km nearer and
// several decibels better off, but the whole model is already stated as a best
// case, and an uncertainty that flatters an answer is not one anybody needs
// protecting from.
//
// The two radii add: the worst arrangement puts each end on the far side of
// its own circle, and the widened separation is priced at free space,
// 20 log10((d + slop) / d). That makes this a floor and not a bound. Moving a
// node five kilometres can also put a ridge in the path, and diffraction over
// a ridge that may or may not be there has no closed form - so a large slack
// is a reason to go and survey the position rather than a correction to apply
// and carry on.
func PositionSlackDB(distKm, aUncertaintyKm, bUncertaintyKm float64) float64 {
	slop := aUncertaintyKm + bUncertaintyKm
	if slop <= 0 || distKm <= 0 {
		return 0
	}
	return 20 * math.Log10((distKm+slop)/distKm)
}

// OneWay is the margin at b for a transmission from a, with the band the two
// nodes' position uncertainty gives it.
//
// Called once per direction, exactly as OneWayDB is: the two directions of a
// link are different answers and collapsing them into one is the mistake this
// whole package is arranged to prevent.
func OneWay(a, b scenario.Node, lossDB float64) Margin {
	distKm := geo.DistanceKm(a.Position.Lat, a.Position.Lon, b.Position.Lat, b.Position.Lon)
	return Margin{
		DB:              OneWayDB(a, b, lossDB),
		PositionSlackDB: PositionSlackDB(distKm, a.UncertaintyKm, b.UncertaintyKm),
	}
}
