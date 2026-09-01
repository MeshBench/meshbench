package validate_test

import (
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/rf/geo"
	"github.com/MeshBench/meshbench/internal/study/validate"
	"github.com/MeshBench/meshbench/internal/world/provider"
)

// ring is flat ground with a circular ridge around a centre point, so every
// path out of that centre crosses high ground.
//
// Obstructed on purpose. Over open ground at 869 MHz these stations have forty
// decibels of margin to spare, and a link that far above the noise reports at
// the modem's SNR ceiling however it is pointed - which would hide the very
// thing these tests are about behind a register width. Real networks are
// terrain limited, and so is this one.
type ring struct {
	lat, lon                        float64
	baseM, peakM, radiusKm, widthKm float64
}

func (r ring) ElevationM(lat, lon float64) (float64, bool) {
	x := (geo.DistanceKm(r.lat, r.lon, lat, lon) - r.radiusKm) / r.widthKm
	return r.baseM + r.peakM*math.Exp(-x*x), true
}

func hills() ring {
	return ring{lat: 56.70, lon: -3.90, baseM: 100, peakM: 400, radiusKm: 4, widthKm: 1.5}
}

// aim gives one station a beam pointed at another: 14 dBi on the nose and 20 dB
// down off the back, which is an unremarkable yagi.
func aim(st map[string]validate.Station, station, target string) map[string]validate.Station {
	s, at := st[station], st[target]
	s.Antenna = antenna.Mounted{
		Pattern:    antenna.Yagi{GainDBiPeak: 14, BeamwidthDeg: 40, FrontToBackDB: 20},
		BearingDeg: geo.BearingDeg(s.Lat, s.Lon, at.Lat, at.Lon),
	}
	st[station] = s
	return st
}

func residualTo(t *testing.T, rep validate.Report, receiver string) float64 {
	t.Helper()
	for _, r := range rep.Residuals {
		if r.To == receiver {
			return r.ResidualDB
		}
	}
	t.Fatalf("no residual for %s in a report of %d", receiver, rep.Used)
	return 0
}

// The reason a station carries an antenna rather than a number. A residual is
// the one figure in this project that is supposed to be about the propagation
// model, and gain credited to a beam aimed somewhere else lands in it as
// unexplained path loss: it gets calibrated in, and the model is left carrying
// the blame for how somebody pointed a mast.
//
// Both receivers hear the same packet at the same reported SNR, and the only
// thing that changes between the two runs is where the origin's beam looks.
func TestPointingIsChargedToTheAntennaNotTheModel(t *testing.T) {
	obs := []provider.Reception{rx("rx-a", "p1", -5), rx("rx-b", "p1", -5)}
	atA, err := validate.Compare(obs, aim(stations(), "origin", "rx-a"), hills(), params())
	if err != nil {
		t.Fatal(err)
	}
	atB, err := validate.Compare(obs, aim(stations(), "origin", "rx-b"), hills(), params())
	if err != nil {
		t.Fatal(err)
	}
	if atA.Used != 2 || atB.Used != 2 {
		t.Fatalf("setup: %d and %d usable observations, want two each", atA.Used, atB.Used)
	}

	// Turning the beam from one receiver to the other has to move both
	// residuals by the antenna's front-to-back ratio, and in opposite
	// directions: the receiver it turns away from is suddenly heard better
	// than predicted, and the one it turns towards worse.
	if moved := residualTo(t, atB, "rx-a") - residualTo(t, atA, "rx-a"); moved < 15 {
		t.Errorf("turning the beam off rx-a moved its residual by %.1f dB; "+
			"pointing is not reaching the prediction", moved)
	}
	if moved := residualTo(t, atA, "rx-b") - residualTo(t, atB, "rx-b"); moved < 15 {
		t.Errorf("turning the beam off rx-b moved its residual by %.1f dB; "+
			"the two pairs are being given one bearing", moved)
	}
}

// Each end is pointed on its own bearing, and the two are opposite. A receiver
// evaluated on the transmitter's bearing rather than its own is looking out of
// the back of its own antenna, which is a mistake worth twenty decibels and
// invisible in an omnidirectional network.
func TestEachEndIsPointedOnItsOwnBearing(t *testing.T) {
	obs := []provider.Reception{rx("rx-a", "p1", -5)}

	towards, err := validate.Compare(obs, aim(stations(), "rx-a", "origin"), hills(), params())
	if err != nil {
		t.Fatal(err)
	}

	// The same yagi at the same station, turned to look away from the origin.
	backwards := aim(stations(), "rx-a", "origin")
	turned := backwards["rx-a"]
	turned.Antenna.BearingDeg = math.Mod(turned.Antenna.BearingDeg+180, 360)
	backwards["rx-a"] = turned

	away, err := validate.Compare(obs, backwards, hills(), params())
	if err != nil {
		t.Fatal(err)
	}
	if moved := residualTo(t, away, "rx-a") - residualTo(t, towards, "rx-a"); moved < 15 {
		t.Errorf("turning the receiving antenna around moved its residual by %.1f dB; "+
			"the receiver is not being evaluated towards the transmitter", moved)
	}
}
