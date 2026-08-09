// Package pathview answers "why did that link miss".
//
// A margin in decibels says a link failed. It does not say whether the fix is
// three metres of mast, a different site, or nothing at all — and those are the
// only answers anyone can act on. This package produces the side-on view that
// distinguishes them: terrain, the line of sight, the first Fresnel zone, and
// the one obstruction that decides the path.
//
// The Fresnel zone is the part people leave out. A path can have clear line of
// sight and still behave nothing like free space, because the wave needs room
// around the sight line as well as along it. The usual engineering rule is 60%
// of the first Fresnel radius clear; below that, diffraction starts to matter,
// and a view that draws only the sight line makes that invisible.
package pathview

import (
	"fmt"
	"math"

	"github.com/A13xB0/meshcoresim/internal/terrain"
)

// Terrain supplies ground elevation.
type Terrain interface {
	ElevationM(lat, lon float64) (float64, bool)
}

// Sample is one point along the path.
type Sample struct {
	DistM float64

	// GroundM is the terrain, and BulgedM is the terrain with the earth's
	// curvature added — which is what the radio actually has to clear. Both are
	// kept because a view that silently plots one as the other cannot be
	// checked against a map.
	GroundM  float64
	BulgedM  float64
	LOSm     float64
	FresnelM float64 // first Fresnel radius here

	// ClearanceM is how far the bulged terrain sits below the sight line.
	// Negative means it is above it.
	ClearanceM float64
}

// CutThrough is the whole analysis.
type CutThrough struct {
	Samples    []Sample
	DistanceKm float64
	FreqMHz    float64
	TxAltM     float64
	RxAltM     float64

	// Worst is the sample that decides the path — the largest intrusion into
	// the Fresnel zone, not simply the highest ground. A tall hill next to the
	// transmitter matters less than a low one in the middle, because the
	// Fresnel zone is widest at mid-path.
	Worst      int
	WorstF1Pct float64 // clearance as a percentage of the first Fresnel radius

	Blocked bool
}

// Analyse walks the path.
func Analyse(t Terrain, fromLat, fromLon, fromHeightM, toLat, toLon, toHeightM, freqMHz float64, samples int) (CutThrough, error) {
	if freqMHz <= 0 {
		return CutThrough{}, fmt.Errorf("pathview: no frequency")
	}
	if samples < 8 {
		samples = 200
	}
	fromGround, ok := t.ElevationM(fromLat, fromLon)
	if !ok {
		return CutThrough{}, fmt.Errorf("pathview: no terrain at the near end")
	}
	toGround, ok := t.ElevationM(toLat, toLon)
	if !ok {
		return CutThrough{}, fmt.Errorf("pathview: no terrain at the far end")
	}

	distKm := haversineKm(fromLat, fromLon, toLat, toLon)
	if distKm <= 0 {
		return CutThrough{}, fmt.Errorf("pathview: both ends are the same place")
	}
	total := distKm * 1000

	c := CutThrough{
		DistanceKm: distKm, FreqMHz: freqMHz,
		TxAltM: fromGround + fromHeightM,
		RxAltM: toGround + toHeightM,
		Worst:  -1,
	}

	worstRatio := math.Inf(1)
	for i := 0; i <= samples; i++ {
		f := float64(i) / float64(samples)
		lat := fromLat + (toLat-fromLat)*f
		lon := fromLon + (toLon-fromLon)*f
		ground, ok := t.ElevationM(lat, lon)
		if !ok {
			return CutThrough{}, fmt.Errorf("pathview: terrain runs out %.1f km along the path", f*distKm)
		}

		d1, d2 := f*total, (1-f)*total
		s := Sample{
			DistM:   d1,
			GroundM: ground,
			LOSm:    c.TxAltM + (c.RxAltM-c.TxAltM)*f,
		}
		// The ends have no curvature and no Fresnel radius; both are zero there
		// by definition rather than by special case.
		if d1 > 0 && d2 > 0 {
			s.BulgedM = ground + terrain.EarthBulgeM(d1, d2)
			s.FresnelM = terrain.FresnelRadiusM(d1, d2, freqMHz)
		} else {
			s.BulgedM = ground
		}
		s.ClearanceM = s.LOSm - s.BulgedM
		c.Samples = append(c.Samples, s)

		// Ranked by clearance as a fraction of the Fresnel radius, not by
		// absolute height. The Fresnel zone is widest at mid-path, so a low
		// hill in the middle can matter more than a taller one near an end —
		// which is exactly the judgement a picture is needed for.
		if s.FresnelM <= 0 {
			continue
		}
		if ratio := s.ClearanceM / s.FresnelM; ratio < worstRatio {
			worstRatio, c.Worst = ratio, i
		}
	}

	if c.Worst >= 0 {
		w := c.Samples[c.Worst]
		c.WorstF1Pct = w.ClearanceM / w.FresnelM * 100
		c.Blocked = w.ClearanceM < 0
	}
	return c, nil
}

// Verdict is the sentence to put under the picture.
//
// Three outcomes, because there are three different actions. Blocked means
// move or raise. Intruding means the path works but not like free space, and a
// few metres may fix it. Clear means the loss is elsewhere — power, antenna, or
// simply distance — and no amount of mast will help.
func (c CutThrough) Verdict() string {
	if c.Worst < 0 {
		return "Path too short to analyse."
	}
	w := c.Samples[c.Worst]
	atKm := w.DistM / 1000

	switch {
	case c.Blocked:
		return fmt.Sprintf(
			"BLOCKED. Terrain at %.1f km sits %.0f m above the line of sight once earth "+
				"curvature is included. Raising an antenna by about that much, or moving to "+
				"clear it, is what changes this — more power will not.",
			atKm, -w.ClearanceM)

	case c.WorstF1Pct < 60:
		return fmt.Sprintf(
			"GRAZING. Line of sight is clear by %.0f m at %.1f km, but that is only %.0f%% of "+
				"the first Fresnel radius (%.0f m). Below 60%% a path stops behaving like free "+
				"space and starts losing to diffraction, so this link will be worse than a "+
				"budget alone suggests — and a few more metres of height would buy a lot.",
			w.ClearanceM, atKm, c.WorstF1Pct, w.FresnelM)

	default:
		return fmt.Sprintf(
			"CLEAR. The tightest point is %.1f km along, with %.0f m of clearance — %.0f%% of "+
				"the first Fresnel radius. Terrain is not what limits this path; if it does not "+
				"work, the answer is in the link budget rather than the profile.",
			atKm, w.ClearanceM, c.WorstF1Pct)
	}
}

// Extent is the height range to draw, with a little air above and below.
func (c CutThrough) Extent() (lo, hi float64) {
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, s := range c.Samples {
		lo = math.Min(lo, s.BulgedM)
		hi = math.Max(hi, math.Max(s.BulgedM, s.LOSm+s.FresnelM))
	}
	if math.IsInf(lo, 1) {
		return 0, 1
	}
	pad := math.Max(20, (hi-lo)*0.1)
	return lo - pad, hi + pad
}

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const rEarth = 6371.0
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * rEarth * math.Asin(math.Min(1, math.Sqrt(a)))
}
