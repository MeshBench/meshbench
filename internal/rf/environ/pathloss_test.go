package environ

import (
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/terrain"
)

// townProvider hands back one fixed town whatever box it is asked for; the
// crossing tests do the real filtering.
type townProvider struct{ blds []Building }

func (t townProvider) Buildings(_, _, _, _ float64) []Building { return t.blds }

// town builds rows of 6 m boxes straddling latitude 56, spaced spacingDeg
// apart in longitude, which is the shape the UK dataset actually has: every
// footprint at envgen's default height because no height was published.
func town(westLon, eastLon, spacingDeg float64) townProvider {
	var blds []Building
	for lon := westLon; lon < eastLon; lon += spacingDeg {
		blds = append(blds, Building{
			Footprint: [][2]float64{
				{55.9990, lon}, {56.0010, lon},
				{56.0010, lon + spacingDeg/3}, {55.9990, lon + spacingDeg/3},
			},
			HeightM: 6, Material: MatBrick, HeightSource: "default",
		})
	}
	return townProvider{blds: blds}
}

// townPath is a 25 km hop straight down the town's high street, both
// antennas 2 m above a 50 m plateau, so every rooftop stands above the ray.
const (
	townWestLon  = -4.40
	townEastLon  = -4.00
	townPathM    = 24908.0
	townTxAslM   = 52.0
	townRxAslM   = 52.0
	townFreqMHz  = 869.525
	townEndInset = 0.002
)

func townIndex(t *testing.T, p townProvider) *PathIndex {
	t.Helper()
	ix := NewPathIndex(p, bareGround{}, 55.9, -4.5, 56.1, -3.9)
	if ix.Buildings() != len(p.blds) {
		t.Fatalf("index holds %d of %d buildings", ix.Buildings(), len(p.blds))
	}
	return ix
}

// A town is one obstacle, not one obstacle per house. Charging a knife edge
// and a wall per crossed footprint priced a 23 km path across Glasgow at
// 2,235 dB, which inverts the direction of every other error in the model:
// it makes the simulator harsher than the air rather than kinder.
func TestATownSaturatesRatherThanAccumulating(t *testing.T) {
	p := town(townWestLon+townEndInset, townEastLon-townEndInset, 0.0006)
	ix := townIndex(t, p)
	sc := &PathScratch{}
	got := ix.PathLossDB(sc, 56.0, townWestLon, townTxAslM,
		56.0, townEastLon, townRxAslM, townPathM, townFreqMHz)

	// What the old rule charged: every crossing's knife edge and one wall.
	var naive float64
	for _, o := range ix.ObstructionsOnPath(sc, 56.0, townWestLon, 56.0, townEastLon) {
		mid := (o.EnterFrac + o.ExitFrac) / 2
		d1 := townPathM * mid
		v := terrain.FresnelParameter(o.TopM-townTxAslM, d1, townPathM-d1, townFreqMHz)
		naive += terrain.KnifeEdgeDB(v) + MaterialLossDB(o.Material, townFreqMHz)
	}
	if naive < 2000 {
		t.Fatalf("the fixture is too kind: the unbounded rule charged only %.0f dB", naive)
	}
	if got > 80 {
		t.Fatalf("%d crossings priced at %.1f dB; a town is meant to saturate", len(p.blds), got)
	}
	t.Logf("%d crossings: %.1f dB, where the unbounded rule charged %.0f dB", len(p.blds), got, naive)

	// Twice the buildings on the same ground is one doubling of the screen
	// count, which the settled field prices at 18 log10(2).
	dense := townIndex(t, town(townWestLon+townEndInset, townEastLon-townEndInset, 0.0003))
	denser := dense.PathLossDB(sc, 56.0, townWestLon, townTxAslM,
		56.0, townEastLon, townRxAslM, townPathM, townFreqMHz)
	if step := denser - got; step < 0 || step > 7 {
		t.Fatalf("doubling the rows added %.1f dB; 18 log10(2) is 5.4", step)
	}
}

// The engine walks every crossing on a path and the coverage raster cannot
// afford to, so the two must be held to the same answer by the price rather
// than by an argument about which crossings matter. A silent disagreement
// here means the map shows a network the packets do not experience.
func TestEngineAndCoveragePriceATownAlike(t *testing.T) {
	p := town(townWestLon+townEndInset, townEastLon-townEndInset, 0.0006)
	ix := townIndex(t, p)
	sc := &PathScratch{}
	sp := ix.Station(56.0, townWestLon)
	for _, lon := range []float64{-4.00, -4.10, -4.28, -4.37, -4.396} {
		distM := townPathM * (lon - townWestLon) / (townEastLon - townWestLon)
		full := ix.PathLossDB(sc, 56.0, townWestLon, townTxAslM,
			56.0, lon, townRxAslM, distM, townFreqMHz)
		ends := ix.PathLossNearEndsDB(sc, 56.0, townWestLon, townTxAslM,
			56.0, lon, townRxAslM, distM, townFreqMHz)
		station := sp.LossDB(sc, true, townTxAslM, 56.0, lon, townRxAslM, distM, townFreqMHz)
		direct := PathBuildingLossDB(p, bareGround{}, 56.0, townWestLon, townTxAslM,
			56.0, lon, townRxAslM, distM, townFreqMHz)
		if full <= 0 {
			t.Fatalf("cell at %.3f: a path down a high street priced nothing", lon)
		}
		if math.Abs(full-ends) > 1e-9 || math.Abs(full-station) > 1e-9 ||
			math.Abs(full-direct) > 1e-9 {
			t.Fatalf("cell at %.3f: engine %.6f dB, near-ends %.6f dB, "+
				"station view %.6f dB, direct %.6f dB", lon, full, ends, station, direct)
		}
	}
}

// The aperture is an argument about rooftops, so it must not swallow a
// structure that is not one. Both halves have to find it, too: the raster
// walks the ends of a path and would never have looked here.
func TestATowerMidPathIsPricedByBoth(t *testing.T) {
	tower := townProvider{blds: []Building{{
		Footprint: [][2]float64{
			{55.995, -4.201}, {56.005, -4.201}, {56.005, -4.199}, {55.995, -4.199}},
		HeightM: 70, Material: MatConcrete,
	}}}
	ix := townIndex(t, tower)
	sc := &PathScratch{}
	sp := ix.Station(56.0, townWestLon)
	full := ix.PathLossDB(sc, 56.0, townWestLon, townTxAslM,
		56.0, townEastLon, townRxAslM, townPathM, townFreqMHz)
	if full < 10 {
		t.Fatalf("a 70 m tower squarely mid-path priced %.2f dB", full)
	}
	ends := ix.PathLossNearEndsDB(sc, 56.0, townWestLon, townTxAslM,
		56.0, townEastLon, townRxAslM, townPathM, townFreqMHz)
	station := sp.LossDB(sc, false, townTxAslM, 56.0, townEastLon,
		townRxAslM, townPathM, townFreqMHz)
	if math.Abs(full-ends) > 1e-9 || math.Abs(full-station) > 1e-9 {
		t.Fatalf("engine %.6f dB, near-ends %.6f dB, station view %.6f dB",
			full, ends, station)
	}
	t.Logf("a 70 m tower 11 km from either end: %.1f dB", full)
}

// One building cannot be capped at the price of its own two walls, or a
// tower block costs what a garden shed's frontage costs. The route through
// pays for the inside as well, so the route over the roof wins and keeps
// rising with height.
func TestADeepBuildingIsNotCappedByItsWalls(t *testing.T) {
	deep := func(h float64) townProvider {
		return townProvider{blds: []Building{{
			Footprint: [][2]float64{
				{55.99, -4.21}, {56.01, -4.21}, {56.01, -4.19}, {55.99, -4.19}},
			HeightM: h, Material: MatConcrete,
		}}}
	}
	sc := &PathScratch{}
	prev := 0.0
	for _, h := range []float64{30, 60, 120, 240} {
		got := townIndex(t, deep(h)).PathLossDB(sc, 56.0, townWestLon, townTxAslM,
			56.0, townEastLon, townRxAslM, townPathM, townFreqMHz)
		if got-prev < 2 {
			t.Fatalf("a %.0f m block priced %.2f dB against %.2f dB for the one "+
				"below it; the walls are capping the roof", h, got, prev)
		}
		prev = got
	}
	// Well past two concrete walls, which is where the cap used to sit.
	if prev < 25 {
		t.Fatalf("a 240 m concrete block across the path priced only %.1f dB", prev)
	}
}

// Buildings the ray clears cost nothing, and the model must say so however
// many of them there are - the whole dataset stands at one default height,
// so "the antennas are above roof level" is the question it can answer.
func TestATownUnderTheRayCostsNothing(t *testing.T) {
	p := town(townWestLon+townEndInset, townEastLon-townEndInset, 0.0006)
	ix := townIndex(t, p)
	sc := &PathScratch{}
	// Both ends 150 m above the plateau: every 6 m roof is far below the ray.
	if got := ix.PathLossDB(sc, 56.0, townWestLon, 200,
		56.0, townEastLon, 200, townPathM, townFreqMHz); got != 0 {
		t.Fatalf("a town 144 m below the ray charged %.3f dB", got)
	}
}

// The price must not step as geometry moves, or a raster draws an edge
// across the map that no town put there. Counting screens as integers does
// step, by 18 log10((n+1)/n) each time a rooftop crosses the shadow
// boundary, which is why a screen fades into the count instead.
//
// Sampled alone a steep slope and a step look the same, so this refines the
// sampling: a slope's largest drop shrinks with the interval and a step's
// does not.
func TestBuildingLossIsContinuousAsTheRayLifts(t *testing.T) {
	p := town(townWestLon+townEndInset, townEastLon-townEndInset, 0.0006)
	ix := townIndex(t, p)
	sc := &PathScratch{}
	worstDrop := func(stepM float64) float64 {
		prev := ix.PathLossDB(sc, 56.0, townWestLon, 52-stepM,
			56.0, townEastLon, 52-stepM, townPathM, townFreqMHz)
		var worst float64
		for asl := 52.0; asl <= 120; asl += stepM {
			got := ix.PathLossDB(sc, 56.0, townWestLon, asl,
				56.0, townEastLon, asl, townPathM, townFreqMHz)
			if got > prev+1e-9 {
				t.Fatalf("lifting both antennas to %.2f m raised the loss "+
					"to %.3f dB from %.3f", asl, got, prev)
			}
			worst = math.Max(worst, prev-got)
			prev = got
		}
		return worst
	}
	coarse := worstDrop(0.5)
	fine := worstDrop(0.02)
	if fine > coarse/10 {
		t.Fatalf("worst drop %.3f dB at half a metre and %.3f dB at 2 cm: "+
			"a drop that will not sample away is a step", coarse, fine)
	}
	t.Logf("worst drop %.3f dB per half metre, %.3f dB per 2 cm", coarse, fine)
}
