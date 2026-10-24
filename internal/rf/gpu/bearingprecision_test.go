package gpu

import (
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/geo"
)

// The fold kernel's bearing, carried out here in float32 with the worst trig
// Vulkan allows - sin and cos each out by 2^-11 - holds to the CPU twin.
//
// No CI runner has a GPU, so the kernel itself is only ever compared on
// whatever device someone owns. This is the arithmetic of coverage.wgsl's
// bearing_deg, psin, pcos, psin_lat and atan2p written out in Go, so the
// property can be checked everywhere: on an Intel UHD 620 the textbook form
// put a beam's bearing nearly nine degrees out, because its x term cancels
// two numbers near 0.46 down to about a thousandth and the trig error
// survives whole.
func TestTheFoldBearingSurvivesTheLeastAccurateTrig(t *testing.T) {
	const stLat, stLon = 56.6, -4.0 // the directional station in the fold comparison
	worstNew, worstOld := 0.0, 0.0
	for _, sinErr := range []float32{-trigErr, trigErr} {
		for _, cosErr := range []float32{-trigErr, trigErr} {
			sinD := func(x float32) float32 { return float32(math.Sin(float64(x))) + sinErr }
			cosD := func(x float32) float32 { return float32(math.Cos(float64(x))) + cosErr }
			for r := range 96 {
				for c := range 96 {
					lat := 56.5 + (float64(r)+0.5)*0.5/96
					lon := -4.2 + (float64(c)+0.5)*1.0/96
					if math.Abs(lat-stLat) < 1e-9 && math.Abs(lon-stLon) < 1e-9 {
						continue
					}
					want := geo.BearingDeg(stLat, stLon, lat, lon)
					worstNew = math.Max(worstNew, angleDiff(want,
						float64(bearingNew32(stLat, stLon, float32(lat), float32(lon), sinD, cosD))))
					worstOld = math.Max(worstOld, angleDiff(want,
						float64(bearingOld32(stLat, stLon, float32(lat), float32(lon), sinD, cosD))))
				}
			}
		}
	}
	t.Logf("worst bearing error with trig out by 2^-11: textbook form %.3f deg, kernel's form %.4f deg",
		worstOld, worstNew)
	// What is left is float32 coordinates, not trig: a cell a quarter of a
	// kilometre from the mast is a few float32 steps of latitude away, and
	// that alone is 0.03 degrees, falling to under 0.01 by 2 km. On the fold
	// test's beam, 12 dB over 180 degrees, 0.05 degrees is 0.005 dB against a
	// tolerance of 0.05.
	if worstNew > 0.05 {
		t.Errorf("the kernel's bearing is %.4f degrees out with the least accurate trig Vulkan permits", worstNew)
	}
}

// trigErr is the absolute error Vulkan permits a driver's sin and cos.
const trigErr = 1.0 / 2048

func angleDiff(a, b float64) float64 {
	d := math.Mod(math.Abs(a-b), 360)
	return math.Min(d, 360-d)
}

// series is coverage.wgsl's psin, pcos and psin_lat, over a driver's sin.
type series struct{ sinD func(float32) float32 }

func (k series) psin(x float32) float32 {
	if x > math.Pi/4 || x < -math.Pi/4 {
		return k.sinD(x)
	}
	x2 := x * x
	return x * (1 - x2/6*(1-x2/20*(1-x2/42*(1-x2/72*(1-x2/110)))))
}

func (k series) pcos(x float32) float32 {
	s := k.psin(x * 0.5)
	return 1 - 2*s*s
}

func (k series) psinLat(x float32) float32 { return 2 * k.psin(x*0.5) * k.pcos(x*0.5) }

// bearingNew32 is coverage.wgsl's bearing_deg.
func bearingNew32(lat1, lon1, lat2, lon2 float32, sinD, _ func(float32) float32) float32 {
	const rad = float32(math.Pi / 180)
	k := series{sinD}
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	c2 := k.pcos(lat2 * rad)
	h := k.psin(dLon * 0.5)
	y := k.psin(dLon) * c2
	x := k.psin(dLat) + 2*k.psinLat(lat1*rad)*c2*h*h
	bg := atan2p32(y, x) / rad
	if bg < 0 {
		bg += 360
	}
	return bg
}

// haversine32 is coverage.wgsl's haversine_km.
func haversine32(lat1, lon1, lat2, lon2 float32, sinD func(float32) float32) float32 {
	const rad = float32(math.Pi / 180)
	k := series{sinD}
	sLat := k.psin((lat2 - lat1) * rad / 2)
	sLon := k.psin((lon2 - lon1) * rad / 2)
	a := sLat*sLat + k.pcos(lat1*rad)*k.pcos(lat2*rad)*sLon*sLon
	return 2 * 6371 * float32(math.Asin(math.Min(1, math.Sqrt(float64(a)))))
}

// The kernel's distance holds too. Half a cell's latitude difference is a few
// ten-thousandths of a radian, the size of the error Vulkan permits sin, and
// with the driver's trig the old form put a cell 50 km away 7 km out.
func TestTheFoldDistanceSurvivesTheLeastAccurateTrig(t *testing.T) {
	const stLat, stLon = 56.6, -4.0
	worst := 0.0
	for _, sinErr := range []float32{-trigErr, trigErr} {
		sinD := func(x float32) float32 { return float32(math.Sin(float64(x))) + sinErr }
		for r := range 96 {
			for c := range 96 {
				lat := 56.5 + (float64(r)+0.5)*0.5/96
				lon := -4.2 + (float64(c)+0.5)*1.0/96
				want := geo.DistanceKm(stLat, stLon, lat, lon)
				got := float64(haversine32(stLat, stLon, float32(lat), float32(lon), sinD))
				worst = math.Max(worst, math.Abs(got-want))
			}
		}
	}
	t.Logf("worst distance error with trig out by 2^-11: %.2f m", worst*1000)
	// Float32 coordinates again: a few metres near the mast, a few hundredths
	// of a dB of path loss at worst.
	if worst > 0.01 {
		t.Errorf("the kernel's distance is %.1f m out with the least accurate trig Vulkan permits", worst*1000)
	}
}

// bearingOld32 is the textbook form the kernel used before.
func bearingOld32(lat1, lon1, lat2, lon2 float32, sinD, cosD func(float32) float32) float32 {
	const rad = float32(math.Pi / 180)
	y := sinD((lon2-lon1)*rad) * cosD(lat2*rad)
	x := cosD(lat1*rad)*sinD(lat2*rad) - sinD(lat1*rad)*cosD(lat2*rad)*cosD((lon2-lon1)*rad)
	bg := float32(math.Atan2(float64(y), float64(x))) / rad
	if bg < 0 {
		bg += 360
	}
	return bg
}

// atan2p32 is coverage.wgsl's atan2p.
func atan2p32(y, x float32) float32 {
	ax, ay := float32(math.Abs(float64(x))), float32(math.Abs(float64(y)))
	mx := max(ax, ay)
	if mx == 0 {
		return 0
	}
	z := min(ax, ay) / mx
	z2 := z * z
	a := z * (0.9998660 + z2*(-0.3302995+z2*(0.1801410+z2*(-0.0851330+z2*0.0208351))))
	if ay > ax {
		a = math.Pi/2 - a
	}
	if x < 0 {
		a = math.Pi - a
	}
	if y < 0 {
		a = -a
	}
	return a
}
