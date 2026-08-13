package coverage

import "math"

// The pair matrix: every node's path loss to every other, over a rasterised
// height grid.
//
// This is the CPU twin of the pairs kernel (ADR-0004): the same construction
// as GridLossCPU with the raster cell replaced by another node. It exists so
// the GPU has something to be tested against, and so a machine without a
// usable GPU loses time rather than the feature.

// PairNode is one end of a path: where it is, and how far above the ground its
// antenna sits.
type PairNode struct {
	Lat, Lon float64
	AGLm     float64
}

// PairParams are the things every pair shares.
type PairParams struct {
	FreqMHz float64
	// StepM is how far apart the profile samples are, and StepsCap bounds a
	// long path so it cannot cost more than it is worth. Both match the
	// engine's own profile, so the two answers are about the same shape of
	// path.
	StepM    float64
	StepsCap int
}

// NoDataLoss marks a pair the terrain could not answer for.
const NoDataLoss = float32(3.4e38)

// PairLossCPU fills an n by n matrix; only the upper triangle is written, and
// the rest is left at zero, exactly as the kernel leaves it.
func PairLossCPU(g HeightGrid, nodes []PairNode, p PairParams) []float32 {
	n := len(nodes)
	out := make([]float32, n*n)
	for a := 0; a < n; a++ {
		for b := a + 1; b < n; b++ {
			out[a*n+b] = pairLoss(g, nodes[a], nodes[b], p)
		}
	}
	return out
}

func pairLoss(g HeightGrid, na, nb PairNode, p PairParams) float32 {
	distKm := haversineKm(na.Lat, na.Lon, nb.Lat, nb.Lon)
	if distKm <= 0 {
		return NoDataLoss
	}
	ga, okA := g.At(na.Lat, na.Lon)
	gb, okB := g.At(nb.Lat, nb.Lon)
	if !okA || !okB {
		return NoDataLoss
	}

	d := distKm * 1000
	lambda := 299.792458 / p.FreqMHz
	txAlt := ga + na.AGLm
	rxAlt := gb + nb.AGLm
	slope := (rxAlt - txAlt) / d

	steps := int(math.Max(2, math.Floor(d/p.StepM)))
	if steps > p.StepsCap {
		steps = p.StepsCap
	}

	maxFromTx, maxFromRx, worstV := math.Inf(-1), math.Inf(-1), math.Inf(-1)
	seen := false
	for i := 1; i < steps; i++ {
		f := float64(i) / float64(steps)
		h, ok := g.At(na.Lat+(nb.Lat-na.Lat)*f, na.Lon+(nb.Lon-na.Lon)*f)
		if !ok {
			return NoDataLoss
		}
		d1 := d * f
		d2 := d - d1
		hb := h + d1*d2/(2*4.0/3.0*6371000)
		if s := (hb - txAlt) / d1; s > maxFromTx {
			maxFromTx = s
		}
		if s := (hb - rxAlt) / d2; s > maxFromRx {
			maxFromRx = s
		}
		hLos := hb - (txAlt + slope*d1)
		if v := hLos * math.Sqrt(2*d/(lambda*d1*d2)); v > worstV {
			worstV = v
		}
		seen = true
	}

	fspl := 32.44 + 20*math.Log10(distKm) + 20*math.Log10(p.FreqMHz)
	if !seen {
		return float32(fspl)
	}

	var v float64
	if maxFromTx < slope {
		v = worstV
	} else {
		db := (rxAlt - txAlt + maxFromRx*d) / (maxFromTx + maxFromRx)
		if db <= 0 || db >= d {
			return float32(fspl)
		}
		h := txAlt + maxFromTx*db - (txAlt + slope*db)
		v = h * math.Sqrt(2*d/(lambda*db*(d-db)))
	}
	loss := 0.0
	if v > -0.78 {
		loss = 6.9 + 20*math.Log10(math.Sqrt((v-0.1)*(v-0.1)+1)+v-0.1)
	}
	if loss > 0 {
		loss += (1 - math.Exp(-loss/6)) * (10 + 0.02*d/1000)
	}
	return float32(fspl + loss)
}
