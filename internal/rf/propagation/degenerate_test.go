package propagation

import (
	"math"
	"testing"
)

// levelTerrain answers everywhere, at one height.
type levelTerrain struct{ m float64 }

func (t levelTerrain) ElevationM(_, _ float64) (float64, bool) { return t.m, true }

// A grid of no cells knows nothing, and has to say so as a number. Known over
// zero is NaN, every comparison against NaN is false, and the caller gates its
// "these tiles are not downloaded" warning on the fraction being small - so
// the emptiest grid there is was the one grid that could never raise it.
func TestAZeroSizedGridKnowsNothing(t *testing.T) {
	for _, size := range [][2]int{{0, 0}, {0, 8}, {8, 0}, {-4, 8}} {
		g, frac := RasteriseHeights(levelTerrain{100}, 56, 56.4, -3.6, -3.0, size[0], size[1])
		if math.IsNaN(frac) {
			t.Errorf("a %dx%d grid reported NaN known, which no warning can test",
				size[0], size[1])
		}
		if frac != 0 {
			t.Errorf("a %dx%d grid reported %v of its cells known", size[0], size[1], frac)
		}
		if len(g.Heights) != 0 {
			t.Errorf("a %dx%d grid allocated %d heights", size[0], size[1], len(g.Heights))
		}
	}
}

// A pair with no length between its ends: log10(0) is -Inf, and a loss of -Inf
// is an infinite margin, which is the most confident answer the budget can
// produce out of the least information there is.
func TestAZeroLengthPairHasAFiniteLoss(t *testing.T) {
	heights := make([]float32, 32)
	for i := range heights {
		heights[i] = float32(100 + i)
	}
	var p PairProfiles
	p.Add(heights, 0, 10, 1.5)
	p.Add(heights, -5, 10, 1.5)
	p.Add(heights, 8000, 10, 1.5)

	got := ProfilePairLossCPU(p, 869.618)
	for i, l := range got[:2] {
		if math.IsInf(float64(l), 0) || math.IsNaN(float64(l)) {
			t.Errorf("pair %d over no distance: loss %v", i, l)
		}
		if l != 0 {
			t.Errorf("pair %d over no distance: loss %v dB, want the grid path's 0", i, l)
		}
	}
	// The pair that has a length still gets a real answer.
	if got[2] <= 0 || math.IsInf(float64(got[2]), 0) {
		t.Errorf("an 8 km pair priced at %v dB", got[2])
	}
}

// foldAt runs one station over a small grid and hands back the cell the
// station stands in, its neighbour, and the margin a zero loss would have
// bought - which is the number the answer must stay under.
func foldAt(t *testing.T, mastAltM float64) (self, next FoldSlot, ceilingDB float64) {
	t.Helper()
	g := flatGrid(56.0, 56.4, -3.6, -3.0, 32, 32)
	p := GridLossParams{
		StAltM: mastAltM, RasterW: 8, RasterH: 8,
		South: 56.0, North: 56.4, West: -3.6, East: -3.0,
		RemoteHeightM: 1.5, FreqMHz: 869.618, Steps: 32,
	}
	// Exactly the centre of cell (4, 4), so the distance to it is exactly zero
	// - which is the only way this branch is ever reached.
	const sx, sy = 4, 4
	p.StLat = p.North - (p.North-p.South)*(sy+0.5)/float64(p.RasterH)
	p.StLon = p.West + (p.East-p.West)*(sx+0.5)/float64(p.RasterW)

	b := StationBudget{TxPowerDBm: 22, SensitivityDBm: -124,
		RemoteTxDBm: 20, RemoteGainDBi: 0, RemoteSensitivityDBm: -124}
	gt := SampleGains(func(_, _ float64) float64 { return 2.15 })
	cells := p.RasterW * p.RasterH
	best := NewFoldSlots(cells)
	second := NewFoldSlots(cells)
	served := make([]uint32, cells)
	FoldStationCPU(GridLossCPU(g, p), g, p, b, gt, best, second, served)

	idx := sy*p.RasterW + sx
	return best[idx], best[idx+1],
		b.TxPowerDBm + 2.15 + b.RemoteGainDBi - b.RemoteSensitivityDBm
}

// The pixel the mast stands in is where the margin is highest, and the fold
// used to hard-code it to zero: a dark cell at every station, on the one cell
// an operator checks first. It must carry a real margin instead - better than
// its neighbours, and short of the margin a zero path loss would invent.
func TestTheStationsOwnCellCarriesItsRealMargin(t *testing.T) {
	self, next, ceiling := foldAt(t, 220)

	if self.Station != 0 {
		t.Fatalf("the station's own cell was not folded: %+v", self)
	}
	if math.IsInf(float64(self.MinDB), 0) || math.IsNaN(float64(self.MinDB)) {
		t.Fatalf("the station's own cell: %+v", self)
	}
	if self.MinDB <= 0 {
		t.Errorf("the mast cannot reach the ground it stands on: %.1f dB", self.MinDB)
	}
	if self.MinDB <= next.MinDB {
		t.Errorf("the station's own cell is worse than its neighbour: %.1f dB against %.1f dB",
			self.MinDB, next.MinDB)
	}
	if float64(self.MinDB) >= ceiling {
		t.Errorf("the station's own cell claims %.1f dB, which a zero path loss "+
			"would not beat (%.1f dB)", self.MinDB, ceiling)
	}
	// Both directions, because a cell that does not say which way it closed is
	// wrong even when the arithmetic is right.
	if self.OutDB <= 0 || self.InDB <= 0 {
		t.Errorf("the station's own cell closes one way only: out %.1f dB, in %.1f dB",
			self.OutDB, self.InDB)
	}
}

// A taller mast is a longer path to the handheld at its foot, so the margin
// there falls. It is a small effect and the wrong sign would be invisible on a
// map, which is exactly why it is asserted.
func TestATallerMastIsFurtherFromItsOwnFoot(t *testing.T) {
	low, _, _ := foldAt(t, 120)
	high, _, _ := foldAt(t, 620)
	if !(high.MinDB < low.MinDB) {
		t.Errorf("a 520 m taller mast did not cost its own cell anything: "+
			"%.2f dB against %.2f dB", high.MinDB, low.MinDB)
	}
}
