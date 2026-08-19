package coverage

import (
	"math"
	"testing"
)

func flatGrid(south, north, west, east float64, w, h int) HeightGrid {
	g := HeightGrid{South: south, North: north, West: west, East: east, W: w, H: h,
		Heights: make([]float32, w*h)}
	for i := range g.Heights {
		g.Heights[i] = 100
	}
	return g
}

func testStations() []Endpoint {
	return []Endpoint{
		{Name: "near", Lat: 56.00, Lon: -3.00, HeightAGLm: 10, TxPowerDBm: 22, SensitivityDBm: -124},
		{Name: "far", Lat: 56.20, Lon: -3.40, HeightAGLm: 10, TxPowerDBm: 22, SensitivityDBm: -124},
	}
}

func testOpts() Options {
	return Options{RemoteHeightAGLm: 1.5, RemoteTxPowerDBm: 20,
		RemoteSensitivityDBm: -124, ProfileStepM: 120}
}

// The direct pass must agree with the reference construction it replaced:
// every station rastered in full, then combined. Same grid, same cells,
// same best server - only the wall-clock differs.
func TestBestServerMatchesRasterThenCombine(t *testing.T) {
	g := flatGrid(55.95, 56.25, -3.5, -2.9, 40, 30)
	stations := testStations()
	o := testOpts()

	direct := &Raster{South: g.South, North: g.North, West: g.West, East: g.East,
		Width: 24, Height: 18, FreqMHz: 869.618}
	combined := BestServer(g, stations, direct, o, nil, nil)

	var rasters []*Raster
	for _, st := range stations {
		r := &Raster{South: g.South, North: g.North, West: g.West, East: g.East,
			Width: 24, Height: 18, FreqMHz: 869.618,
			Cells: make([]Cell, 24*18)}
		if err := Compute(st, gridTerrain{g}, r, o); err != nil {
			t.Fatal(err)
		}
		rasters = append(rasters, r)
	}
	ref, err := Combine(rasters)
	if err != nil {
		t.Fatal(err)
	}

	for i := range ref.Cells {
		if ref.Cells[i].NoData != direct.Cells[i].NoData {
			t.Fatalf("cell %d nodata: direct %v ref %v", i, direct.Cells[i].NoData, ref.Cells[i].NoData)
		}
		if ref.Cells[i].NoData {
			continue
		}
		dm := math.Min(direct.Cells[i].OutboundMarginDB, direct.Cells[i].InboundMarginDB)
		rm := ref.BestMarginDB[i]
		if math.Abs(dm-rm) > 1e-9 {
			t.Fatalf("cell %d best margin: direct %.6f ref %.6f", i, dm, rm)
		}
		if combined.ServingCount[i] != ref.ServingCount[i] {
			t.Fatalf("cell %d serving: direct %d ref %d", i, combined.ServingCount[i], ref.ServingCount[i])
		}
		if combined.BestNode[i] != ref.BestNode[i] {
			t.Fatalf("cell %d best node: direct %d ref %d", i, combined.BestNode[i], ref.BestNode[i])
		}
	}
}

// A building on the path must cost the cell exactly what the callback says:
// the raster the operator sees has to move when the environment loads.
func TestBestServerPricesTheExtraLoss(t *testing.T) {
	g := flatGrid(55.98, 56.02, -3.05, -2.95, 20, 20)
	stations := testStations()[:1]
	o := testOpts()
	mk := func(extra func(int, float64, float64, float64, float64, float64) float64) *Raster {
		r := &Raster{South: g.South, North: g.North, West: g.West, East: g.East,
			Width: 10, Height: 10, FreqMHz: 869.618}
		BestServer(g, stations, r, o, extra, nil)
		return r
	}
	bare := mk(nil)
	walled := mk(func(_ int, _, _, _, _, _ float64) float64 { return 40 })
	moved := 0
	for i := range bare.Cells {
		if bare.Cells[i].NoData {
			continue
		}
		d := bare.Cells[i].OutboundMarginDB - walled.Cells[i].OutboundMarginDB
		if math.Abs(d-40) < 1e-9 {
			moved++
		}
	}
	if moved == 0 {
		t.Fatal("40 dB of building cost no cell anything")
	}
}

// Row zero of a loss grid is the NORTH edge - Raster.LatLonAt's convention.
// The kernel and its CPU twin once agreed with each other south-up, passed
// their equivalence test, and painted every raster upside down for the
// first consumer that trusted them. A station at the south edge must see
// its loss GROW toward row zero.
func TestGridLossRowZeroIsNorth(t *testing.T) {
	g := flatGrid(56.0, 56.5, -3.6, -3.0, 32, 32)
	p := GridLossParams{
		StLat: 56.01, StLon: -3.3, StAltM: 120,
		RasterW: 16, RasterH: 16,
		South: 56.0, North: 56.5, West: -3.6, East: -3.0,
		RemoteHeightM: 1.5, FreqMHz: 869.618, Steps: 64,
	}
	losses := GridLossCPU(g, p)
	north := losses[0*16+8]
	south := losses[15*16+8]
	if !(north > south) {
		t.Fatalf("station at the south edge: north row loss %.1f, south row %.1f - "+
			"the grid is upside down", north, south)
	}
}
