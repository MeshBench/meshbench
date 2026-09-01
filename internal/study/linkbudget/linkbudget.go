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

	"github.com/MeshBench/meshbench/internal/rf/dsp"
	"github.com/MeshBench/meshbench/internal/rf/geo"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// DefaultBandwidthHz and DefaultSF are what a node that has not said gets.
const (
	DefaultBandwidthHz = 250e3
	DefaultNoiseFigDB  = 6
)

// GainDBi is a node's antenna gain towards a far end, with its feedline loss
// already taken off.
//
// Directional in azimuth, boresight in elevation, and the difference between
// the two halves is the whole point. The bearing from one node to the other
// falls exactly out of two positions the scenario already holds, and a yagi or
// a sector is twenty decibels or more down off its boresight: quoting the peak
// across azimuth is not a rough answer, it is a confident wrong one, and wrong
// towards the optimistic, which is the direction this tool refuses to be wrong
// in. The elevation angle is the opposite case. On a terrestrial path it is a
// fraction of a degree, and this package is handed two nodes and a number of
// decibels with no ground beneath them to derive one from, so reading the
// elevation plane at boresight claims no precision the geometry cannot
// support.
//
// It follows that the gain two nodes credit each other is not one number: A to
// B and B to A are different bearings, and on a beam they are, in effect,
// different antennas.
func GainDBi(n, towards scenario.Node) float64 {
	if n.Antenna.Pattern == nil {
		return -n.Antenna.FeedlineDB
	}
	if n.Position == towards.Position {
		// Two nodes at one point have no direction between them, and a planned
		// site checked against itself arrives here. The best case is the least
		// surprising answer, and the only one that is not an arbitrary bearing.
		return n.Antenna.Pattern.PeakDBi() - n.Antenna.FeedlineDB
	}
	return n.Antenna.GainAlongDBi(geo.BearingDeg(
		n.Position.Lat, n.Position.Lon, towards.Position.Lat, towards.Position.Lon))
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
//
// The best case, and only that: it prices the two nodes exactly where they say
// they are. Where either was imported rather than surveyed, OneWay carries the
// same figure with the band that position uncertainty gives it, and that is
// the one to decide on.
//
// Each end's antenna is evaluated towards the other, so a beam pointed
// somewhere else pays for it here rather than in a footnote.
func OneWayDB(a, b scenario.Node, lossDB float64) float64 {
	return a.TxPowerDBm + GainDBi(a, b) - lossDB + GainDBi(b, a) - SensitivityDBm(b)
}

// MarginDB is the weaker of the two directions.
//
// The weaker, always, because a link that works one way is not a link: the
// case worth showing is precisely the one where a mast can be heard by a
// handheld that it cannot hear back.
//
// A best case in the same way OneWayDB is, and for the same reason.
func MarginDB(a, b scenario.Node, lossDB float64) float64 {
	return math.Min(OneWayDB(a, b, lossDB), OneWayDB(b, a, lossDB))
}

// Term is one named quantity in a budget, in decibels.
type Term struct {
	Name string
	DB   float64
}

// Terms breaks a one-way budget into the lines that make it up, in the order
// a signal meets them.
//
// The sum of the terms is exactly OneWayDB. That is not a coincidence to be
// checked by eye - a breakdown that does not add up to the number beside it is
// worse than no breakdown, because it invites somebody to trust the wrong one.
func Terms(a, b scenario.Node, lossDB float64) []Term {
	return []Term{
		{"transmit power", a.TxPowerDBm},
		{"antenna, transmitting", GainDBi(a, b)},
		{"path loss", -lossDB},
		{"antenna, receiving", GainDBi(b, a)},
		{"receiver sensitivity", -SensitivityDBm(b)},
	}
}
