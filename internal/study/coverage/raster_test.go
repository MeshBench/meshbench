package coverage_test

import (
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/study/coverage"
)

// A ridge running north-south at a fixed longitude, with flat ground either
// side. Analytic rather than sampled so the tests assert against geometry they
// can state, not against whatever the code happened to produce.
type ridge struct {
	atLon   float64
	widthDe float64
	heightM float64
	// noDataWestOf marks everything west of a longitude as uncovered, standing
	// in for the edge of a downloaded tile.
	noDataWestOf float64
}

func (r ridge) ElevationM(lat, lon float64) (float64, bool) {
	if r.noDataWestOf != 0 && lon < r.noDataWestOf {
		return 0, false
	}
	d := math.Abs(lon - r.atLon)
	if d > r.widthDe {
		return 100, true
	}
	return 100 + r.heightM*(1-d/r.widthDe), true
}

type flat struct{ h float64 }

func (f flat) ElevationM(_, _ float64) (float64, bool) { return f.h, true }

func station() coverage.Endpoint {
	return coverage.Endpoint{
		Name: "GB7XYZ", Lat: 56.7, Lon: -3.9,
		HeightAGLm: 20, TxPowerDBm: 27, SensitivityDBm: -130,
	}
}

func opts() coverage.Options {
	return coverage.Options{
		RemoteHeightAGLm: 1.5, RemoteTxPowerDBm: 14, RemoteGainDBi: 0,
		RemoteSensitivityDBm: -130, ProfileStepM: 100,
	}
}

func grid() *coverage.Raster {
	return &coverage.Raster{
		South: 56.6, North: 56.8, West: -4.1, East: -3.7,
		Width: 40, Height: 20, FreqMHz: 869.525,
	}
}

// The whole reason this package returns two margins. A 27 dBm repeater on a
// 20 m mast and a 14 dBm handheld at 1.5 m do not have the same link, and the
// cells where one closes and the other does not are the useful answer.
func TestReachIsAsymmetric(t *testing.T) {
	// A fine radial rather than a coarse grid. LoRa over flat ground closes
	// comfortably in both directions until earth curvature shuts the path, and
	// then it shuts fast: the 13 dB between the two ends is worth only a couple
	// of kilometres out there. On a 4 km grid that band falls between cells and
	// the asymmetry is invisible — which is a resolution artefact, not physics.
	r := &coverage.Raster{
		South: 56.7, North: 56.71, West: -3.9, East: -1.9,
		Width: 1200, Height: 1, FreqMHz: 869.525,
	}
	if err := coverage.Compute(station(), flat{100}, r, opts()); err != nil {
		t.Fatal(err)
	}

	oneWay, both := 0, 0
	for x := 0; x < r.Width; x++ {
		c := r.At(x, 0)
		if c.NoData {
			continue
		}
		if c.OneWay() {
			oneWay++
		}
		if c.Workable() {
			both++
		}
		// Outbound is 13 dB stronger over the same terrain, so it can never be
		// the weaker direction.
		if c.OutboundMarginDB < c.InboundMarginDB-1e-9 {
			t.Fatalf("at x=%d the 14 dBm handheld out-reached the 27 dBm repeater", x)
		}
		if d := c.OutboundMarginDB - c.InboundMarginDB; math.Abs(d-13) > 1e-9 {
			t.Fatalf("at x=%d the directions differ by %.3f dB, not the 13 dB between the ends", x, d)
		}
	}
	if oneWay == 0 {
		t.Error("no cell where the repeater is heard but cannot hear — the asymmetry is not being computed")
	}
	t.Logf("%d cells two-way, %d cells one-way only, out of %d", both, oneWay, r.Width)
}

// A ridge between the two ends must cost something. Without this the
// diffraction path is not actually being walked and the raster is free-space
// with extra steps.
func TestTerrainBlocks(t *testing.T) {
	obstructed := grid()
	if err := coverage.Compute(station(), ridge{atLon: -3.8, widthDe: 0.03, heightM: 400}, obstructed, opts()); err != nil {
		t.Fatal(err)
	}
	clear := grid()
	if err := coverage.Compute(station(), flat{100}, clear, opts()); err != nil {
		t.Fatal(err)
	}

	// A cell on the far side of the ridge from the station.
	x, y := 35, 10
	blocked, open := obstructed.At(x, y), clear.At(x, y)
	if blocked.PathLossDB <= open.PathLossDB+3 {
		t.Errorf("a 400 m ridge cost only %.1f dB", blocked.PathLossDB-open.PathLossDB)
	}
	if blocked.OutboundMarginDB >= open.OutboundMarginDB {
		t.Error("margin did not fall behind the ridge")
	}
}

// Missing terrain is ignorance, not a result. Filling it with sea level would
// draw confident coverage across water the DEM never described.
func TestNoDataIsNotNoCoverage(t *testing.T) {
	r := grid()
	if err := coverage.Compute(station(), ridge{atLon: -3.8, widthDe: 0.03, heightM: 200, noDataWestOf: -4.0}, r, opts()); err != nil {
		t.Fatal(err)
	}
	var noData int
	for i := range r.Cells {
		if r.Cells[i].NoData {
			noData++
		}
		if r.Cells[i].NoData && r.Cells[i].Workable() {
			t.Fatal("a cell with no data reported a workable link")
		}
	}
	if noData == 0 {
		t.Error("the uncovered strip was not marked as no-data")
	}
}

func TestComputeRefusesAStationOffTheMap(t *testing.T) {
	s := station()
	s.Lon = -4.5
	err := coverage.Compute(s, ridge{atLon: -3.8, widthDe: 0.03, heightM: 100, noDataWestOf: -4.2}, grid(), opts())
	if err == nil {
		t.Fatal("a station with no terrain under it was accepted")
	}
}

// Combining is where a network stops being a set of nodes. The count of serving
// nodes matters as much as the coverage: one repeater and four look the same on
// a map and are not the same network.
func TestCombineCountsRedundancy(t *testing.T) {
	var rasters []*coverage.Raster
	for _, lon := range []float64{-4.0, -3.9, -3.8} {
		s := station()
		s.Lon = lon
		r := grid()
		if err := coverage.Compute(s, flat{100}, r, opts()); err != nil {
			t.Fatal(err)
		}
		rasters = append(rasters, r)
	}

	c, err := coverage.Combine(rasters)
	if err != nil {
		t.Fatal(err)
	}
	gaps, known := c.GapCells()
	t.Logf("%d/%d cells unserved, redundancy %.2f, %d single-point-of-failure cells",
		gaps, known, c.Redundancy(), c.SinglePointOfFailure())

	if c.Redundancy() <= 1 {
		t.Error("three overlapping stations gave no redundancy at all")
	}
	for i := range c.Cells {
		if c.ServingCount[i] > 0 && c.BestNode[i] < 0 {
			t.Fatalf("cell %d is served but has no best node", i)
		}
	}
}

// The margin describing a link is the weaker direction. Taking the better one
// would call a link workable on the strength of the half that was never in
// doubt — which is the same mistake as reporting one direction as "in range".
func TestCombineUsesTheWeakerDirection(t *testing.T) {
	r := grid()
	if err := coverage.Compute(station(), flat{100}, r, opts()); err != nil {
		t.Fatal(err)
	}
	c, err := coverage.Combine([]*coverage.Raster{r})
	if err != nil {
		t.Fatal(err)
	}
	for i, cell := range c.Cells {
		if cell.NoData {
			continue
		}
		want := math.Min(cell.OutboundMarginDB, cell.InboundMarginDB)
		if math.Abs(c.BestMarginDB[i]-want) > 1e-9 {
			t.Fatalf("cell %d: best margin %.2f, weaker direction %.2f", i, c.BestMarginDB[i], want)
		}
	}
}

func TestCombineRefusesMismatchedGrids(t *testing.T) {
	a, b := grid(), grid()
	b.Width = 10
	if err := coverage.Compute(station(), flat{100}, a, opts()); err != nil {
		t.Fatal(err)
	}
	if err := coverage.Compute(station(), flat{100}, b, opts()); err != nil {
		t.Fatal(err)
	}
	if _, err := coverage.Combine([]*coverage.Raster{a, b}); err == nil {
		t.Fatal("rasters over different grids were combined")
	}
}
