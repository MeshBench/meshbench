package antenna

import (
	"math"
	"testing"
)

func TestDipoleHasHorizonGainAndAxialNulls(t *testing.T) {
	d := Dipole{}
	if g := d.GainDBi(0, 0); math.Abs(g-2.15) > 0.05 {
		t.Errorf("dipole at the horizon = %.2f dBi, want 2.15", g)
	}
	// Straight up is the null — the reason a vertical dipole is deaf overhead.
	if g := d.GainDBi(0, 90); g > -20 {
		t.Errorf("dipole overhead = %.1f dBi, expected a deep null", g)
	}
	// Omnidirectional in azimuth.
	for _, az := range []float64{0, 90, 180, -90} {
		if math.Abs(d.GainDBi(az, 0)-2.15) > 0.05 {
			t.Errorf("dipole not omnidirectional at azimuth %.0f", az)
		}
	}
}

// More gain means a narrower vertical beam. This is the trade an operator is
// actually making when they buy more dBi, and the model must express it.
func TestCollinearGainCostsBeamwidth(t *testing.T) {
	low, high := Collinear{GainDBiPeak: 3}, Collinear{GainDBiPeak: 9}
	if high.GainDBi(0, 0) <= low.GainDBi(0, 0) {
		t.Error("higher-gain collinear should win at the horizon")
	}
	// At 10 degrees below the horizon the high-gain antenna should have given
	// away more, because its beam is squashed.
	lowDrop := low.GainDBi(0, 0) - low.GainDBi(0, -10)
	highDrop := high.GainDBi(0, 0) - high.GainDBi(0, -10)
	if highDrop <= lowDrop {
		t.Errorf("high-gain antenna dropped %.1f dB off-boresight, low-gain %.1f — "+
			"gain must cost beamwidth", highDrop, lowDrop)
	}
}

func TestYagiIsDirectional(t *testing.T) {
	y := Yagi{GainDBiPeak: 12, BeamwidthDeg: 50, FrontToBackDB: 20}
	front := y.GainDBi(0, 0)
	back := y.GainDBi(180, 0)
	if math.Abs(front-12) > 0.01 {
		t.Errorf("yagi boresight = %.2f dBi, want 12", front)
	}
	if front-back < 19 {
		t.Errorf("front-to-back only %.1f dB, want ~20", front-back)
	}
}

// The rule the package exists to enforce: the same antenna gives different gain
// toward different neighbours, and a scalar gain field is a bug.
func TestGainIsDirectional(t *testing.T) {
	m := Mounted{
		Pattern:    Yagi{GainDBiPeak: 12, BeamwidthDeg: 50, FrontToBackDB: 20},
		BearingDeg: 90, // pointing east
		FeedlineDB: 1.2,
	}
	east := m.GainTowardsDBi(90, 0)  // straight down the boresight
	west := m.GainTowardsDBi(270, 0) // straight out the back
	if east-west < 19 {
		t.Errorf("east %.1f dBi vs west %.1f dBi — antenna is not directional", east, west)
	}
	if math.Abs(east-(12-1.2)) > 0.01 {
		t.Errorf("boresight gain %.2f did not deduct feedline loss", east)
	}
}

// Asymmetry: the look angle from A to B is not the look angle from B to A once
// elevation is involved, so the two ends of one link see different gains.
func TestLookAngleIsNotSymmetric(t *testing.T) {
	hilltop := Mounted{Pattern: Collinear{GainDBiPeak: 6}, DowntiltDeg: 0}
	valley := Mounted{Pattern: Collinear{GainDBiPeak: 6}, DowntiltDeg: 0}

	// The valley station is 8 degrees below the hilltop; the hilltop is
	// therefore 8 degrees above the valley.
	down := hilltop.GainTowardsDBi(0, -8)
	up := valley.GainTowardsDBi(0, +8)
	if math.Abs(down-up) > 0.01 {
		t.Fatalf("identical antennas should see the same magnitude of off-axis angle")
	}

	// Now tilt the hilltop antenna down, which is what an operator would do.
	hilltop.DowntiltDeg = 8
	improved := hilltop.GainTowardsDBi(0, -8)
	if improved <= down {
		t.Errorf("downtilt of 8 deg did not improve gain toward a station 8 deg below "+
			"(%.2f -> %.2f dBi)", down, improved)
	}
	// And the link is now asymmetric in antenna terms alone.
	if math.Abs(improved-up) < 0.5 {
		t.Error("after downtilt the two ends should no longer see equal gain")
	}
}

func TestCrossPolarisationLoss(t *testing.T) {
	if CrossPolLossDB(Vertical, Vertical) != 0 {
		t.Error("matched polarisation should cost nothing")
	}
	if got := CrossPolLossDB(Vertical, Horizontal); got < 15 {
		t.Errorf("orthogonal linear polarisation cost only %.1f dB", got)
	}
	if got := CrossPolLossDB(Circular, Vertical); math.Abs(got-3) > 0.01 {
		t.Errorf("circular-to-linear = %.1f dB, want the classic 3 dB", got)
	}
}
