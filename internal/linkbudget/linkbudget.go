// Package linkbudget turns a path loss into a margin.
//
// It exists so the map, the budget panel and anything else that draws a link
// cannot disagree about whether that link closes. The arithmetic was
// previously inside the old UI package, which meant the new one either
// imported a user interface to do radio maths or wrote its own copy - and two
// copies of a link budget drift, silently, in the direction of whichever one
// somebody last looked at.
package linkbudget

import (
	"math"

	"github.com/A13xB0/meshcoresim/internal/dsp"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// DefaultBandwidthHz and DefaultSF are what a node that has not said gets.
const (
	DefaultBandwidthHz = 250e3
	DefaultNoiseFigDB  = 6
)

// GainDBi is a node's peak antenna gain with its feedline loss already taken
// off. Peak rather than in-direction: a link drawn on a map has no elevation
// angle to speak of, and pretending to a precision the geometry does not
// support would be worse than being plainly approximate.
func GainDBi(n scenario.Node) float64 {
	if n.Antenna.Pattern == nil {
		return -n.Antenna.FeedlineDB
	}
	return n.Antenna.Pattern.PeakDBi() - n.Antenna.FeedlineDB
}

// BandwidthHz is the receiver bandwidth this node uses.
func BandwidthHz(n scenario.Node) float64 {
	if n.Radio.BandwidthHz > 0 {
		return n.Radio.BandwidthHz
	}
	return DefaultBandwidthHz
}

// NoiseFloorDBm is the thermal floor in this node's bandwidth.
func NoiseFloorDBm(n scenario.Node) float64 {
	return dsp.NoiseFloorDBm(BandwidthHz(n), DefaultNoiseFigDB)
}

// RequiredSNRDB is what the spreading factor needs to decode.
func RequiredSNRDB(n scenario.Node) float64 {
	switch n.Radio.SpreadFactor {
	case 7:
		return -7.5
	case 8:
		return -10
	case 9:
		return -12.5
	case 10:
		return -15
	case 11:
		return -17.5
	case 12:
		return -20
	}
	return -15
}

// SensitivityDBm is the weakest signal this node can decode.
func SensitivityDBm(n scenario.Node) float64 {
	return NoiseFloorDBm(n) + RequiredSNRDB(n)
}

// OneWayDB is the margin at b for a transmission from a, given the path loss
// between them. Positive closes.
func OneWayDB(a, b scenario.Node, lossDB float64) float64 {
	return a.TxPowerDBm + GainDBi(a) - lossDB + GainDBi(b) - SensitivityDBm(b)
}

// MarginDB is the weaker of the two directions.
//
// The weaker, always, because a link that works one way is not a link: the
// case worth showing is precisely the one where a mast can be heard by a
// handheld that it cannot hear back.
func MarginDB(a, b scenario.Node, lossDB float64) float64 {
	return math.Min(OneWayDB(a, b, lossDB), OneWayDB(b, a, lossDB))
}
