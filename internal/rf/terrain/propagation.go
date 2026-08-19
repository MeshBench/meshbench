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

// MultiEdgeLossDB is the terrain diffraction loss over a profile, by the
// Bullington construction with the ITU-R P.526 correction.
//
// NOT single knife-edge. In hamreach a Glen Coe link read 36.8 dB under a
// single-edge model — verdict "works", a handheld through a 1,084 m massif —
// and 120.1 dB once corrected. A single-edge model is not conservative; it is
// wrong.
//
// It was Deygout, and Deygout is the wrong tool here. Deygout decomposes a path
// into a principal edge and recurses either side of it, which is sound when the
// profile has distinct peaks and unsound when it is smooth: a spherical earth
// flattens into a parabola, and recursing on a parabola charges the same
// curvature once per level. Measured on flat ground at 869 MHz, loss stepped
// from 0 dB at 68 km to 34.7 dB at 69 km — a hard edge across a coverage raster
// that no terrain put there.
//
// Bullington reduces the whole profile to one equivalent edge from the steepest
// sight line at each end, so it is continuous by construction and cannot
// double-count. The correction term restores the loss that a single equivalent
// edge under-reads on genuinely multi-edge paths. This is what ITU-R P.1812 and
// P.452 use for exactly this problem.
//
// txH and rxH are antenna heights above ground at each end.
func MultiEdgeLossDB(profile []Point, txH, rxH, freqMHz float64) float64 {
	if len(profile) < 3 || freqMHz <= 0 {
		return 0
	}
	last := len(profile) - 1
	d := profile[last].DistM - profile[0].DistM
	if d <= 0 {
		return 0
	}
	lambda := 299.792458 / freqMHz
	txAlt := profile[0].HeightM + txH
	rxAlt := profile[last].HeightM + rxH

	// Flatten the earth once, against the full path. The bulge belongs to the
	// path as a whole; applying it per sub-path is what made the old model
	// discontinuous.
	flat := make([]Point, len(profile))
	for i, p := range profile {
		d1 := p.DistM - profile[0].DistM
		d2 := profile[last].DistM - p.DistM
		flat[i] = Point{DistM: d1, HeightM: p.HeightM + EarthBulgeM(d1, d2)}
	}

	slopeLOS := (rxAlt - txAlt) / d

	// Steepest sight line from each end to any point on the profile.
	maxFromTx, maxFromRx := -math.MaxFloat64, -math.MaxFloat64
	for i := 1; i < last; i++ {
		d1 := flat[i].DistM
		d2 := d - d1
		if d1 <= 0 || d2 <= 0 {
			continue
		}
		if s := (flat[i].HeightM - txAlt) / d1; s > maxFromTx {
			maxFromTx = s
		}
		if s := (flat[i].HeightM - rxAlt) / d2; s > maxFromRx {
			maxFromRx = s
		}
	}
	if maxFromTx == -math.MaxFloat64 {
		return 0
	}

	var v float64
	if maxFromTx < slopeLOS {
		// Nothing crosses the line of sight. The obstruction, if any, is
		// Fresnel-zone intrusion, so take the worst intrusion along the path.
		v = -math.MaxFloat64
		for i := 1; i < last; i++ {
			d1, d2 := flat[i].DistM, d-flat[i].DistM
			if d1 <= 0 || d2 <= 0 {
				continue
			}
			h := flat[i].HeightM - (txAlt + slopeLOS*d1)
			if vi := h * math.Sqrt(2*d/(lambda*d1*d2)); vi > v {
				v = vi
			}
		}
	} else {
		// The two sight lines cross above the terrain: that intersection is the
		// single equivalent edge the whole profile is reduced to.
		db := (rxAlt - txAlt + maxFromRx*d) / (maxFromTx + maxFromRx)
		if db <= 0 || db >= d {
			return 0
		}
		h := txAlt + maxFromTx*db - (txAlt + slopeLOS*db)
		v = h * math.Sqrt(2*d/(lambda*db*(d-db)))
	}

	uncorrected := KnifeEdgeDB(v)
	if uncorrected <= 0 {
		return 0
	}
	// ITU-R P.526 correction. One equivalent edge under-reads a path with
	// several real ones; the term grows with path length and switches off
	// smoothly as the diffraction itself vanishes, so continuity survives.
	return uncorrected + (1-math.Exp(-uncorrected/6))*(10+0.02*d/1000)
}

// Edge is one obstruction, with the loss it contributes on its own.
type Edge struct {
	DistM float64
	TopM  float64
	// ClearanceM is how far below the line of sight the edge sits; negative
	// means it stands above it.
	ClearanceM float64
	LossDB     float64
}

// Edges decomposes a path into the obstructions that shape it.
//
// The point is a sentence somebody can act on: not "153 dB of loss" but "that
// ridge at 4.2 km costs you 31 dB". MultiEdgeLossDB reduces a profile to one
// equivalent edge because that is what the propagation model needs; this is the
// same construction, kept in pieces because the *explanation* needs the pieces.
//
// The losses are per-edge Bullington contributions and do not sum to
// MultiEdgeLossDB — that carries the P.526 correction for multiple edges, which
// belongs to the path rather than to any one of them. Presented as a
// decomposition, never as an addition.
func Edges(profile []Point, txH, rxH, freqMHz float64) []Edge {
	if len(profile) < 3 || freqMHz <= 0 {
		return nil
	}
	last := len(profile) - 1
	d := profile[last].DistM - profile[0].DistM
	if d <= 0 {
		return nil
	}
	lambda := 299.792458 / freqMHz
	txAlt := profile[0].HeightM + txH
	rxAlt := profile[last].HeightM + rxH
	slope := (rxAlt - txAlt) / d

	flat := make([]Point, len(profile))
	for i, p := range profile {
		d1 := p.DistM - profile[0].DistM
		flat[i] = Point{DistM: d1, HeightM: p.HeightM + EarthBulgeM(d1, d-d1)}
	}

	// Local maxima of intrusion into the Fresnel zone. A hill is one edge, not
	// the forty samples that describe it, so a peak must beat its neighbours
	// over a window rather than merely be positive.
	const window = 6
	var out []Edge
	for i := 1; i < last; i++ {
		d1, d2 := flat[i].DistM, d-flat[i].DistM
		if d1 <= 0 || d2 <= 0 {
			continue
		}
		h := flat[i].HeightM - (txAlt + slope*d1)
		v := h * math.Sqrt(2*d/(lambda*d1*d2))
		if v <= -0.78 {
			continue // below the knife-edge model's floor: no loss to report
		}
		peak := true
		for j := i - window; j <= i+window; j++ {
			if j < 1 || j > last-1 || j == i {
				continue
			}
			dj1, dj2 := flat[j].DistM, d-flat[j].DistM
			if dj1 <= 0 || dj2 <= 0 {
				continue
			}
			hj := flat[j].HeightM - (txAlt + slope*dj1)
			if hj*math.Sqrt(2*d/(lambda*dj1*dj2)) > v {
				peak = false
				break
			}
		}
		if !peak {
			continue
		}
		loss := KnifeEdgeDB(v)
		if loss <= 0.1 {
			continue
		}
		out = append(out, Edge{
			DistM: profile[0].DistM + d1, TopM: profile[i].HeightM,
			ClearanceM: -h, LossDB: loss,
		})
	}
	return out
}
