package terrain

import (
	"math"
	"testing"
)

// Checked against an independently known figure, not against its own
// plausibility. This formula was once wrong by a factor of 1,000 in hamreach and
// survived review precisely because nobody did this.
//
// Midpoint of a 20 km path: d1 = d2 = 10,000 m.
//
//	h = 1e8 / (2 · 4/3 · 6,371,000) = 5.885 m
//
// Sanity: a 20 km path over water needs ~6 m of extra clearance at midpoint,
// which matches the standard microwave-link rule of thumb.
func TestEarthBulgeAgainstKnownFigure(t *testing.T) {
	got := EarthBulgeM(10_000, 10_000)
	if math.Abs(got-5.885) > 0.01 {
		t.Errorf("earth bulge at 20 km midpoint = %.3f m, want 5.885 m", got)
	}
	// Order-of-magnitude guard: the factor-of-1000 bug would show here.
	if got < 1 || got > 20 {
		t.Fatalf("bulge %.3f m is not physically plausible for a 20 km path", got)
	}
	// Bulge is zero at the endpoints and maximal at the middle.
	if EarthBulgeM(0, 20_000) != 0 {
		t.Error("bulge at an endpoint should be zero")
	}
	if EarthBulgeM(5_000, 15_000) >= got {
		t.Error("bulge should be maximal at the midpoint")
	}
}

// Fresnel radius at the midpoint of a 10 km path at 869 MHz.
//
//	λ = 0.345 m, r = sqrt(0.345 · 5000 · 5000 / 10000) = 29.4 m
func TestFresnelRadius(t *testing.T) {
	got := FresnelRadiusM(5000, 5000, 869)
	if math.Abs(got-29.37) > 0.3 {
		t.Errorf("Fresnel radius = %.2f m, want ~29.37 m", got)
	}
	// Wider at lower frequency — the reason 433 MHz tolerates more clutter.
	if FresnelRadiusM(5000, 5000, 433) <= got {
		t.Error("lower frequency should give a wider Fresnel zone")
	}
}

func TestFSPL(t *testing.T) {
	// 1 km at 869 MHz: 32.44 + 0 + 58.78 = 91.2 dB
	if got := FSPLdB(1, 869); math.Abs(got-91.22) > 0.05 {
		t.Errorf("FSPL(1km, 869MHz) = %.2f, want 91.22", got)
	}
	// Doubling distance costs exactly 6.02 dB.
	if d := FSPLdB(2, 869) - FSPLdB(1, 869); math.Abs(d-6.02) > 0.01 {
		t.Errorf("doubling distance cost %.2f dB, want 6.02", d)
	}
}

func TestKnifeEdge(t *testing.T) {
	// Grazing (v=0) is the classic 6 dB.
	if got := KnifeEdgeDB(0); math.Abs(got-6.0) > 0.3 {
		t.Errorf("knife edge at v=0 gave %.2f dB, want ~6", got)
	}
	// Clear of the obstruction: no loss.
	if got := KnifeEdgeDB(-1.0); got != 0 {
		t.Errorf("knife edge well below LOS gave %.2f dB, want 0", got)
	}
	// Deeper obstruction costs monotonically more.
	if KnifeEdgeDB(2) <= KnifeEdgeDB(1) {
		t.Error("loss should increase with obstruction depth")
	}
}

// The Glen Coe case, in miniature: a single massive ridge mid-path must produce
// a loss that says "no path", not one a handheld could overcome.
func TestMassiveRidgeIsNotWorkable(t *testing.T) {
	// 12 km path, both ends at 100 m, a 900 m ridge at the midpoint.
	profile := []Point{}
	for i := 0; i <= 60; i++ {
		d := float64(i) * 200
		h := 100.0
		if i > 20 && i < 40 {
			// smooth ridge peaking at 900 m
			x := float64(i-30) / 10
			h = 100 + 800*math.Exp(-x*x*2)
		}
		profile = append(profile, Point{DistM: d, HeightM: h})
	}
	loss := MultiEdgeLossDB(profile, 12, 2, 869)
	if loss < 30 {
		t.Errorf("900 m ridge across a 12 km path gave only %.1f dB — a single-edge "+
			"model reading 'works' through a massif is the hamreach Glen Coe bug", loss)
	}
	t.Logf("ridge diffraction loss: %.1f dB", loss)
}

// A clear path must cost nothing in diffraction, or every open link is penalised.
func TestClearPathHasNoDiffractionLoss(t *testing.T) {
	profile := []Point{}
	for i := 0; i <= 40; i++ {
		profile = append(profile, Point{DistM: float64(i) * 100, HeightM: 50})
	}
	if loss := MultiEdgeLossDB(profile, 30, 30, 869); loss > 0.5 {
		t.Errorf("flat clear path gave %.2f dB of diffraction loss, want ~0", loss)
	}
}

// Multi-edge must exceed single-edge on a profile with two real obstructions —
// that difference is the entire reason ADR-0005 insists on Deygout.
func TestMultiEdgeExceedsSingleEdge(t *testing.T) {
	profile := []Point{}
	for i := 0; i <= 60; i++ {
		d := float64(i) * 200
		h := 50.0
		for _, peak := range []struct{ at, height float64 }{{18, 400}, {42, 380}} {
			x := (float64(i) - peak.at) / 6
			h += peak.height * math.Exp(-x*x*2)
		}
		profile = append(profile, Point{DistM: d, HeightM: h})
	}
	multi := MultiEdgeLossDB(profile, 10, 10, 869)

	// Single-edge equivalent: only the principal obstruction.
	var worst float64
	d := profile[len(profile)-1].DistM
	for i := 1; i < len(profile)-1; i++ {
		d1 := profile[i].DistM
		d2 := d - d1
		los := (50 + 10) + ((50+10)-(50+10))*(d1/d)
		h := profile[i].HeightM + EarthBulgeM(d1, d2) - los
		if v := FresnelParameter(h, d1, d2, 869); v > worst {
			worst = v
		}
	}
	single := KnifeEdgeDB(worst)
	if multi <= single {
		t.Errorf("multi-edge %.1f dB did not exceed single-edge %.1f dB on a two-ridge path", multi, single)
	}
	t.Logf("two ridges: single-edge %.1f dB, multi-edge %.1f dB", single, multi)
}
