// Package geo is the great-circle arithmetic the rest of the tree does on
// latitude and longitude.
//
// One implementation, because there were eight. Six were the same formula
// differing only in whether the earth's radius was called r or rEarth; one had
// been renamed haversineKmSession purely to stop it colliding with another copy
// in its own package; one more sat in a test. Bearing had two copies of the
// same forward azimuth. None of them disagreed - which is the point. A formula
// copied eight times is not wrong yet, it is waiting to be, and the copy that
// gets the fix is never the one somebody is reading.
//
// Deliberately small. This is spherical arithmetic on a sphere of one radius:
// anything that needs an ellipsoid, a projection or a screen belongs where it
// is used, not here.
package geo

import "math"

// earthKm is the mean radius, which is what a haversine on a sphere can use.
// Every copy this package replaced used the same figure.
const earthKm = 6371.0

const rad = math.Pi / 180

// DistanceKm is the great-circle distance between two points, in kilometres.
//
// The asin argument is clamped because floating point can push it a hair above
// one for antipodal inputs, and NaN out of a distance function surfaces a long
// way from here.
func DistanceKm(lat1, lon1, lat2, lon2 float64) float64 {
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthKm * math.Asin(math.Min(1, math.Sqrt(a)))
}

// BearingDeg is the initial great-circle bearing from the first point to the
// second, in degrees clockwise from north.
//
// Initial, not constant: a great circle's bearing changes along its length, so
// this is the direction to set off in, which is what an antenna pattern wants.
//
// The result is always in [0, 360). The copies this replaced normalised with
// `if b < 0 { b += 360 }`, which returns exactly 360 for a bearing a hair below
// zero - due west along the antimeridian, for one. Nothing indexed a table with
// it unguarded, so nothing was wrong; but "degrees clockwise from north" is a
// contract, and one that holds only for most inputs is the kind of thing found
// by an out-of-range panic rather than by reading.
func BearingDeg(lat1, lon1, lat2, lon2 float64) float64 {
	y := math.Sin((lon2-lon1)*rad) * math.Cos(lat2*rad)
	x := math.Cos(lat1*rad)*math.Sin(lat2*rad) -
		math.Sin(lat1*rad)*math.Cos(lat2*rad)*math.Cos((lon2-lon1)*rad)
	return math.Mod(math.Atan2(y, x)/rad+360, 360)
}

// ElevationDeg is the look angle from one antenna to another: positive when
// the far end is above us, negative when it is below.
//
// The altitudes are absolute - ground plus height above it - and the distance
// is the great-circle one between the two points. Here rather than at each
// caller because it is the other half of the direction an antenna pattern is
// evaluated in, and the two callers that need it are a coverage raster and
// the engine's delivery path: the map and the packet must not be able to
// disagree about the angle at which a repeater sees a node below it.
//
// Flat, deliberately: the Earth's curvature between two points that can hear
// each other is under a tenth of a degree, which is far inside the accuracy
// of any pattern this is fed to.
func ElevationDeg(fromAltM, toAltM, distKm float64) float64 {
	if distKm <= 0 {
		return 0
	}
	return math.Atan2(toAltM-fromAltM, distKm*1000) / rad
}
