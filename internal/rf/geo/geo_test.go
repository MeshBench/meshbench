package geo

import (
	"math"
	"testing"
)

// Reference values, so a change to the formula has to argue with something
// outside this package rather than with itself.
func TestDistanceAgainstKnownPairs(t *testing.T) {
	cases := []struct {
		name                   string
		lat1, lon1, lat2, lon2 float64
		km                     float64
		tolKm                  float64
	}{
		// Edinburgh Castle to Glasgow Cathedral: the scale the simulator is
		// usually asked about.
		{"Edinburgh-Glasgow", 55.9486, -3.1999, 55.8632, -4.2340, 65.5, 0.5},
		// A degree of latitude is the same length anywhere.
		{"one degree of latitude", 0, 0, 1, 0, 111.19, 0.05},
		// A degree of longitude at 60N is half what it is at the equator.
		{"one degree of longitude at 60N", 60, 0, 60, 1, 55.6, 0.1},
		{"same point", 55.9486, -3.1999, 55.9486, -3.1999, 0, 0.000001},
		// Antipodal: half the circumference, and the case where the asin
		// argument can float above one.
		{"antipodal", 0, 0, 0, 180, math.Pi * earthKm, 0.001},
	}
	for _, c := range cases {
		if got := DistanceKm(c.lat1, c.lon1, c.lat2, c.lon2); math.Abs(got-c.km) > c.tolKm {
			t.Errorf("%s: got %.4f km, want %.4f +/- %.4f", c.name, got, c.km, c.tolKm)
		}
	}
}

func TestDistanceIsSymmetric(t *testing.T) {
	a, b := DistanceKm(55.9486, -3.1999, 56.4620, -2.9707), DistanceKm(56.4620, -2.9707, 55.9486, -3.1999)
	if a != b {
		t.Errorf("not symmetric: %v then %v", a, b)
	}
}

func TestBearingCardinals(t *testing.T) {
	cases := []struct {
		name                   string
		lat1, lon1, lat2, lon2 float64
		deg                    float64
	}{
		{"due north", 0, 0, 1, 0, 0},
		{"due east", 0, 0, 0, 1, 90},
		{"due south", 1, 0, 0, 0, 180},
		{"due west", 0, 1, 0, 0, 270},
	}
	for _, c := range cases {
		if got := BearingDeg(c.lat1, c.lon1, c.lat2, c.lon2); math.Abs(got-c.deg) > 0.01 {
			t.Errorf("%s: got %.4f deg, want %.4f", c.name, got, c.deg)
		}
	}
}

// Every bearing lands in [0, 360), which is what the callers index tables with.
func TestBearingIsNormalised(t *testing.T) {
	for lat := -80.0; lat <= 80; lat += 10 {
		for lon := -180.0; lon < 180; lon += 15 {
			b := BearingDeg(0, 0, lat, lon)
			if b < 0 || b >= 360 || math.IsNaN(b) {
				t.Fatalf("bearing to %v,%v is %v", lat, lon, b)
			}
		}
	}
}
