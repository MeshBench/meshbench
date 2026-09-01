package validate

import (
	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/rf/geo"
)

// Station is a node with everything needed to price a link to or from it.
type Station struct {
	Name          string
	Lat, Lon      float64
	UncertaintyKm float64
	HeightAGLm    float64
	TxPowerDBm    float64

	// Antenna is the pattern as it is actually mounted, not a headline figure.
	// A station carrying a scalar gain cannot be pointed, and see
	// gainTowardsDBi for what that costs the one number in this project that
	// is meant to be about the propagation model.
	Antenna antenna.Mounted

	NoiseFigureDB float64
}

// gainTowardsDBi is this station's gain towards another, feedline deducted.
//
// Both ends are placed well enough to be in the residuals at all, so the
// bearing between them is a fact rather than an estimate, and a station on a
// beam is tens of decibels down off it. Crediting the peak would push that
// pointing loss into the residual, where it is indistinguishable from excess
// path loss: it would be calibrated in, and the model would be made to look
// wrong for an antenna somebody aimed at somewhere else. Calibration that
// blames the model for a mast's orientation is worse than no calibration,
// because it is confident.
//
// In elevation the far end is taken as on the boresight, for the opposite
// reason. A real station's tilt is in no feed this imports, so charging one
// would be a guess wearing the clothes of geometry, and the terrestrial look
// angle it would be charged against is a fraction of a degree.
func (s Station) gainTowardsDBi(far Station) float64 {
	if s.Antenna.Pattern == nil {
		return -s.Antenna.FeedlineDB
	}
	return s.Antenna.GainAlongDBi(geo.BearingDeg(s.Lat, s.Lon, far.Lat, far.Lon))
}
