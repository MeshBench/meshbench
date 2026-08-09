// Package terrain turns ground profiles into path loss.
//
// The numbers here were expensive to get right in hamreach and two of them were
// wrong at some point. Both mistakes are pinned by test rather than trusted.
package terrain

import "math"

// EarthRadiusM is the mean earth radius.
const EarthRadiusM = 6_371_000.0

// EffectiveEarthFactor is the standard k = 4/3 refraction factor: the atmosphere
// bends radio slightly downward, so the earth behaves flatter than it is.
const EffectiveEarthFactor = 4.0 / 3.0

// FSPLdB is free-space path loss.
//
//	L = 32.44 + 20 log10(d_km) + 20 log10(f_MHz)
func FSPLdB(distanceKm, freqMHz float64) float64 {
	if distanceKm <= 0 {
		return 0
	}
	return 32.44 + 20*math.Log10(distanceKm) + 20*math.Log10(freqMHz)
}

// EarthBulgeM is how far the earth rises above the chord between two points,
// at a point d1 from one end and d2 from the other, under k-factor refraction.
//
//	h = d1·d2 / (2·k·R)
//
// All distances in metres. This formula was once wrong by a factor of 1,000 in
// hamreach and survived review, because it was only ever checked against its own
// plausibility. Check it against a known figure.
func EarthBulgeM(d1M, d2M float64) float64 {
	return d1M * d2M / (2 * EffectiveEarthFactor * EarthRadiusM)
}

// FresnelRadiusM is the first Fresnel zone radius at a point d1/d2 along a path.
//
//	r = sqrt( λ·d1·d2 / (d1+d2) )
//
// The 60% clearance criterion is what engineers actually reason with, and it is
// what the terrain cut-through view draws.
func FresnelRadiusM(d1M, d2M, freqMHz float64) float64 {
	d := d1M + d2M
	if d <= 0 || freqMHz <= 0 {
		return 0
	}
	lambda := 299.792458 / freqMHz // metres
	return math.Sqrt(lambda * d1M * d2M / d)
}

// FresnelParameter is ITU-R P.526's dimensionless v for a knife edge of height h
// above the line of sight.
//
//	v = h · sqrt( 2·(d1+d2) / (λ·d1·d2) )
func FresnelParameter(hM, d1M, d2M, freqMHz float64) float64 {
	if d1M <= 0 || d2M <= 0 {
		return 0
	}
	lambda := 299.792458 / freqMHz
	return hM * math.Sqrt(2*(d1M+d2M)/(lambda*d1M*d2M))
}

// KnifeEdgeDB is the ITU-R P.526 single knife-edge diffraction loss for
// Fresnel parameter v. Zero below v = -0.78, where the approximation stops
// being valid and the obstruction is not yet blocking.
func KnifeEdgeDB(v float64) float64 {
	if v <= -0.78 {
		return 0
	}
	return 6.9 + 20*math.Log10(math.Sqrt((v-0.1)*(v-0.1)+1)+v-0.1)
}

// Point is one sample of a terrain profile: distance along the path and ground
// height, both metres.
type Point struct {
	DistM   float64
	HeightM float64
}

// MultiEdgeLossDB is Deygout multi-edge diffraction over a profile.
//
// NOT single knife-edge. In hamreach a Glen Coe link read 36.8 dB under a
// single-edge model — verdict "works", a handheld through a 1,084 m massif — and
// 120.1 dB once corrected. A single-edge model is not conservative, it is wrong.
//
// txH and rxH are antenna heights above ground at each end.
func MultiEdgeLossDB(profile []Point, txH, rxH, freqMHz float64) float64 {
	if len(profile) < 3 {
		return 0
	}
	txAlt := profile[0].HeightM + txH
	rxAlt := profile[len(profile)-1].HeightM + rxH
	total := profile[len(profile)-1].DistM - profile[0].DistM
	if total <= 0 {
		return 0
	}
	return deygout(profile, 0, len(profile)-1, txAlt, rxAlt, freqMHz, 0)
}

// deygout finds the principal edge in [lo, hi], takes its loss, then recurses
// into the two sub-paths either side of it. Depth-limited: real profiles rarely
// have more than a handful of significant edges, and unbounded recursion on a
// noisy profile is a way to spend a lot of time on nothing.
func deygout(p []Point, lo, hi int, loAlt, hiAlt, freqMHz float64, depth int) float64 {
	if depth > 3 || hi-lo < 2 {
		return 0
	}
	d := p[hi].DistM - p[lo].DistM
	if d <= 0 {
		return 0
	}

	bestIdx, bestV := -1, -math.MaxFloat64
	for i := lo + 1; i < hi; i++ {
		d1 := p[i].DistM - p[lo].DistM
		d2 := p[hi].DistM - p[i].DistM
		if d1 <= 0 || d2 <= 0 {
			continue
		}
		// Line-of-sight altitude at this point, plus the earth's bulge.
		los := loAlt + (hiAlt-loAlt)*(d1/d)
		h := p[i].HeightM + EarthBulgeM(d1, d2) - los
		v := FresnelParameter(h, d1, d2, freqMHz)
		if v > bestV {
			bestV, bestIdx = v, i
		}
	}
	if bestIdx < 0 || bestV <= -0.78 {
		return 0
	}

	loss := KnifeEdgeDB(bestV)
	// Sub-paths are terminated at the principal edge's own height.
	edgeAlt := p[bestIdx].HeightM
	loss += deygout(p, lo, bestIdx, loAlt, edgeAlt, freqMHz, depth+1)
	loss += deygout(p, bestIdx, hi, edgeAlt, hiAlt, freqMHz, depth+1)
	return loss
}
