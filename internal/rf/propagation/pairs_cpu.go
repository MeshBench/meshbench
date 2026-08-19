package propagation

import "math"

// NoDataLoss marks a pair the terrain could not answer for.
const NoDataLoss = float32(3.4e38)

// The profile-based pair loss: the CPU twin of the pairs kernel (ADR-0004).
//
// PairProfiles is the packed form both sides read: every profile's heights end
// to end, with per-pair offsets, counts, distances and antenna heights. Packed
// once on the CPU from the hot tile cache, so the two computations see
// byte-identical ground.
type PairProfiles struct {
	Heights []float32
	// MetaU is offset, count per pair. MetaF is distance in metres, antenna
	// height A, antenna height B per pair.
	MetaU []uint32
	MetaF []float32
}

// Pairs is how many pairs are packed.
func (p PairProfiles) Pairs() int { return len(p.MetaU) / 2 }

// Add packs one profile and returns its index.
func (p *PairProfiles) Add(heights []float32, distM, aglA, aglB float64) int {
	off := uint32(len(p.Heights))
	p.Heights = append(p.Heights, heights...)
	p.MetaU = append(p.MetaU, off, uint32(len(heights)))
	p.MetaF = append(p.MetaF, float32(distM), float32(aglA), float32(aglB))
	return p.Pairs() - 1
}

// ProfilePairLossCPU computes every packed pair, exactly as the kernel does.
func ProfilePairLossCPU(p PairProfiles, freqMHz float64) []float32 {
	out := make([]float32, p.Pairs())
	for i := range out {
		out[i] = profilePairLoss(p, i, freqMHz)
	}
	return out
}

func profilePairLoss(p PairProfiles, idx int, freqMHz float64) float32 {
	off := int(p.MetaU[idx*2])
	cnt := int(p.MetaU[idx*2+1])
	d := float64(p.MetaF[idx*3])
	aglA := float64(p.MetaF[idx*3+1])
	aglB := float64(p.MetaF[idx*3+2])

	distKm := d / 1000
	fspl := 32.44 + 20*math.Log10(distKm) + 20*math.Log10(freqMHz)
	if cnt < 3 {
		return float32(fspl)
	}

	lambda := 299.792458 / freqMHz
	txAlt := float64(p.Heights[off]) + aglA
	rxAlt := float64(p.Heights[off+cnt-1]) + aglB
	slope := (rxAlt - txAlt) / d
	n := float64(cnt - 1)

	maxFromTx, maxFromRx, worstV := math.Inf(-1), math.Inf(-1), math.Inf(-1)
	for i := 1; i < cnt-1; i++ {
		f := float64(i) / n
		d1 := d * f
		d2 := d - d1
		hb := float64(p.Heights[off+i]) + d1*d2/(2*4.0/3.0*6371000)
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
