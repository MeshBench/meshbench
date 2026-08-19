// The CPU twin of the GPU fold entry point (ADR-0004): margins from a
// sampled gain table, folded into best and second-best slots per cell.
// The gain table is shared with the device rather than the pattern
// function itself, so the two twins interpolate the same numbers and the
// equivalence test can hold them together.
package propagation

import (
	"math"

	"github.com/MeshBench/meshbench/internal/rf/geo"
)

// GainTable is an antenna pattern sampled onto an azimuth-elevation grid,
// az-major per elevation row. Azimuth covers the full circle and wraps;
// elevation is clamped to the sampled band, which is wide enough that
// anything outside it is a cell almost underneath the mast.
type GainTable struct {
	AzN, ElN            int
	ElMinDeg, ElStepDeg float64
	DB                  []float32
}

// SampleGains prices a pattern function onto a table. One degree of
// azimuth and three of elevation over ±30° keeps the bilinear error well
// under a tenth of a decibel for patterns smooth enough to be antennas.
func SampleGains(gainTowardsDBi func(bearingDeg, elevationDeg float64) float64) GainTable {
	const azN, elN, elMin, elStep = 360, 21, -30.0, 3.0
	t := GainTable{AzN: azN, ElN: elN, ElMinDeg: elMin, ElStepDeg: elStep,
		DB: make([]float32, azN*elN)}
	for e := 0; e < elN; e++ {
		for a := 0; a < azN; a++ {
			t.DB[e*azN+a] = float32(gainTowardsDBi(float64(a), elMin+float64(e)*elStep))
		}
	}
	return t
}

// Sample interpolates the table exactly as the kernel does.
func (t GainTable) Sample(bearingDeg, elevationDeg float64) float64 {
	az := bearingDeg - 360*math.Floor(bearingDeg/360)
	fa := az / 360 * float64(t.AzN)
	ia := int(math.Floor(fa)) % t.AzN
	ia1 := (ia + 1) % t.AzN
	ta := fa - math.Floor(fa)
	fe := (elevationDeg - t.ElMinDeg) / t.ElStepDeg
	fe = math.Min(math.Max(fe, 0), float64(t.ElN-1))
	ie := int(math.Floor(fe))
	ie1 := ie + 1
	if ie1 > t.ElN-1 {
		ie1 = t.ElN - 1
	}
	te := fe - float64(ie)
	g0 := float64(t.DB[ie*t.AzN+ia])*(1-ta) + float64(t.DB[ie*t.AzN+ia1])*ta
	g1 := float64(t.DB[ie1*t.AzN+ia])*(1-ta) + float64(t.DB[ie1*t.AzN+ia1])*ta
	return g0*(1-te) + g1*te
}

// StationBudget is what the fold needs to know about one station beyond
// its geometry: the powers and sensitivities on both ends, and which
// station index a win should be recorded as.
type StationBudget struct {
	TxPowerDBm, SensitivityDBm float64
	RemoteTxDBm, RemoteGainDBi float64
	RemoteSensitivityDBm       float64
	Station                    int
}

// FoldSlot is one cell's running winner (or runner-up): the weaker-direction
// margin that ranked it, both directions, and the station it belongs to.
// Station is -1 while empty.
type FoldSlot struct {
	MinDB, OutDB, InDB float32
	Station            int32
}

// NewFoldSlots is a slate of empty slots.
func NewFoldSlots(n int) []FoldSlot {
	s := make([]FoldSlot, n)
	for i := range s {
		s[i] = FoldSlot{MinDB: -math.MaxFloat32, OutDB: -math.MaxFloat32,
			InDB: -math.MaxFloat32, Station: -1}
	}
	return s
}

// FoldStationCPU folds one station's losses into the slots, the kernel's
// exact arithmetic: change one, change both.
func FoldStationCPU(losses []float32, g HeightGrid, p GridLossParams,
	b StationBudget, gt GainTable, best, second []FoldSlot, served []uint32) {
	for y := 0; y < p.RasterH; y++ {
		lat := p.North - (p.North-p.South)*(float64(y)+0.5)/float64(p.RasterH)
		for x := 0; x < p.RasterW; x++ {
			i := y*p.RasterW + x
			loss := float64(losses[i])
			if loss >= float64(NoDataLoss)/2 {
				continue
			}
			lon := p.West + (p.East-p.West)*(float64(x)+0.5)/float64(p.RasterW)
			var mOut, mIn float64
			distKm := geo.DistanceKm(p.StLat, p.StLon, lat, lon)
			if distKm <= 0 {
				mOut, mIn = 0, 0
			} else {
				ground, _ := g.At(lat, lon)
				rxAlt := ground + p.RemoteHeightM
				bearing := geo.BearingDeg(p.StLat, p.StLon, lat, lon)
				elev := math.Atan2(rxAlt-p.StAltM, distKm*1000) * 180 / math.Pi
				gain := gt.Sample(bearing, elev)
				mOut = b.TxPowerDBm + gain - loss + b.RemoteGainDBi - b.RemoteSensitivityDBm
				mIn = b.RemoteTxDBm + b.RemoteGainDBi - loss + gain - b.SensitivityDBm
			}
			if mOut >= 0 && mIn >= 0 {
				served[i]++
			}
			m := float32(math.Min(mOut, mIn))
			cand := FoldSlot{MinDB: m, OutDB: float32(mOut), InDB: float32(mIn),
				Station: int32(b.Station)}
			switch {
			case m > best[i].MinDB:
				second[i] = best[i]
				best[i] = cand
			case m > second[i].MinDB:
				second[i] = cand
			}
		}
	}
}
