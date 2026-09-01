// Package antenna models radiation patterns, orientation and polarisation.
//
// The rule this package exists to enforce: gain is directional. A scalar "gain"
// field is a bug — evaluate the pattern in the true direction to the far end,
// separately for each direction, because the look angle from A to B is not the
// look angle from B to A once elevation is involved.
package antenna

import "math"

// Polarisation of an antenna. Mismatch costs real decibels and is the reason a
// handheld held sideways performs badly.
type Polarisation string

const (
	Vertical   Polarisation = "vertical"
	Horizontal Polarisation = "horizontal"
	Circular   Polarisation = "circular"
)

// CrossPolLossDB is the loss from a polarisation mismatch. Linear-to-linear
// orthogonal is severe; circular-to-linear costs the classic 3 dB.
func CrossPolLossDB(a, b Polarisation) float64 {
	switch {
	case a == b:
		return 0
	case a == Circular || b == Circular:
		return 3.0
	default:
		return 20.0 // vertical vs horizontal, in practice limited by cross-pol rejection
	}
}

// MismatchLossDB is what a pair of mounted antennas lose to each other's
// polarisation, and nothing at all where either has not said what it is.
//
// The pair, not one end: polarisation costs decibels only in relation to
// something else, and CrossPolLossDB alone had no caller for exactly that
// reason. Unstated has to be free rather than orthogonal, because "" against
// "vertical" is a scenario nobody described, and charging it 20 dB would take
// every link in an older network off the air on the strength of a blank field.
func MismatchLossDB(a, b Mounted) float64 {
	if a.Polarisation == "" || b.Polarisation == "" {
		return 0
	}
	return CrossPolLossDB(a.Polarisation, b.Polarisation)
}

// Pattern gives gain in dBi for a direction in the antenna's own frame.
// azimuthDeg is 0 at boresight, elevationDeg positive above the horizon.
type Pattern interface {
	Name() string
	GainDBi(azimuthDeg, elevationDeg float64) float64
	PeakDBi() float64
}

// Isotropic radiates equally in all directions. Physically impossible, and the
// reference every other gain is quoted against.
type Isotropic struct{}

func (Isotropic) Name() string                 { return "isotropic" }
func (Isotropic) GainDBi(_, _ float64) float64 { return 0 }
func (Isotropic) PeakDBi() float64             { return 0 }

// Dipole is a half-wave dipole: omnidirectional in azimuth, with nulls straight
// up and down. 2.15 dBi at the horizon.
type Dipole struct{}

func (Dipole) Name() string     { return "half-wave dipole" }
func (Dipole) PeakDBi() float64 { return 2.15 }
func (Dipole) GainDBi(_, elevationDeg float64) float64 {
	// Standard half-wave pattern in the elevation plane.
	th := (90 - elevationDeg) * math.Pi / 180 // angle from the dipole axis
	s := math.Sin(th)
	if math.Abs(s) < 1e-9 {
		return -60 // the null along the axis; floored rather than -Inf
	}
	f := math.Cos(math.Pi/2*math.Cos(th)) / s
	if f <= 0 {
		return -60
	}
	return 2.15 + 20*math.Log10(f)
}

// Collinear is a vertically stacked omni: more gain at the horizon, bought by
// squashing the pattern vertically. The workhorse repeater antenna.
type Collinear struct {
	// GainDBiPeak is the manufacturer's headline figure, at the horizon.
	GainDBiPeak float64
}

func (c Collinear) Name() string     { return "collinear omni" }
func (c Collinear) PeakDBi() float64 { return c.GainDBiPeak }
func (c Collinear) GainDBi(_, elevationDeg float64) float64 {
	// Beamwidth narrows as gain rises: roughly 78/G degrees between half-power
	// points for a stacked omni, which is the trade an operator is actually
	// making when they buy more dBi.
	bw := 78.0 / math.Max(c.GainDBiPeak, 1)
	x := elevationDeg / (bw / 2)
	return c.GainDBiPeak - 3*x*x
}

// Yagi is a directional beam: gain in one direction, bought everywhere else.
type Yagi struct {
	GainDBiPeak   float64
	BeamwidthDeg  float64 // horizontal half-power beamwidth
	FrontToBackDB float64
}

func (y Yagi) Name() string     { return "yagi" }
func (y Yagi) PeakDBi() float64 { return y.GainDBiPeak }
func (y Yagi) GainDBi(azimuthDeg, elevationDeg float64) float64 {
	az := normaliseDeg(azimuthDeg)
	bw := y.BeamwidthDeg
	if bw <= 0 {
		bw = 50
	}
	f2b := y.FrontToBackDB
	if f2b <= 0 {
		f2b = 20
	}
	// Gaussian main lobe in both planes, floored at the front-to-back ratio so
	// the back is attenuated rather than infinitely bad.
	a := az / (bw / 2)
	e := elevationDeg / (bw / 2)
	g := y.GainDBiPeak - 3*(a*a+e*e)
	if g < y.GainDBiPeak-f2b {
		g = y.GainDBiPeak - f2b
	}
	return g
}

// Mounted is a pattern placed in the world: rotated to a bearing and tilt.
type Mounted struct {
	Pattern      Pattern
	BearingDeg   float64 // compass bearing of boresight
	DowntiltDeg  float64 // positive tilts the beam below the horizon
	Polarisation Polarisation
	FeedlineDB   float64 // positive number, a loss
}

// GainTowardsDBi returns gain in the direction of a far end at the given compass
// bearing and elevation angle, with feedline loss already deducted.
//
// Elevation is signed: positive means the far end is above us.
func (m Mounted) GainTowardsDBi(bearingDeg, elevationDeg float64) float64 {
	relAz := normaliseDeg(bearingDeg - m.BearingDeg)
	// Downtilt aims the beam downward, so a far end below us moves toward
	// boresight rather than away from it.
	relEl := elevationDeg + m.DowntiltDeg
	return m.Pattern.GainDBi(relAz, relEl) - m.FeedlineDB
}

// GainAlongDBi is gain towards a compass bearing with the far end taken to be
// on the boresight in elevation, feedline loss already deducted.
//
// For a caller pricing a terrestrial path from positions alone. The bearing
// between two known points is exact, so azimuth is never in doubt; the
// elevation angle needs both ends' altitudes, and a caller holding a map and a
// path loss does not have them. Inventing a look angle out of nothing would be
// a precision the geometry cannot support, so the elevation plane is read at
// its best and the answer is a stated best case instead.
func (m Mounted) GainAlongDBi(bearingDeg float64) float64 {
	// Backing out the mount's own downtilt is what "on the boresight" means
	// once an antenna is allowed to be tilted: a tilt aims at ground this
	// caller cannot see, so charging for it here would price a beam against
	// geometry nobody in the call chain has.
	return m.GainTowardsDBi(bearingDeg, -m.DowntiltDeg)
}

// normaliseDeg maps an angle to (-180, 180].
func normaliseDeg(d float64) float64 {
	for d > 180 {
		d -= 360
	}
	for d <= -180 {
		d += 360
	}
	return d
}
