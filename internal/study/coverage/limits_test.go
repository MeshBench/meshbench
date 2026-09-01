package coverage_test

import (
	"math"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/propagation"
	"github.com/MeshBench/meshbench/internal/study/coverage"
)

// levelGrid is ground at one height, so a size test measures sizes.
func levelGrid(w, h int) propagation.HeightGrid {
	g := propagation.HeightGrid{South: 56, North: 56.4, West: -3.6, East: -3.0,
		W: w, H: h, Heights: make([]float32, w*h)}
	for i := range g.Heights {
		g.Heights[i] = 100
	}
	return g
}

func mast() coverage.Endpoint {
	return coverage.Endpoint{Name: "mast", Lat: 56.2, Lon: -3.3, HeightAGLm: 20,
		TxPowerDBm: 22, SensitivityDBm: -124}
}

func mastOpts() coverage.Options {
	return coverage.Options{RemoteHeightAGLm: 1.5, RemoteTxPowerDBm: 20,
		RemoteSensitivityDBm: -124, ProfileStepM: 120}
}

// A raster nobody could allocate has to be refused, not attempted. 50000
// square is a hundred gigabytes of cells, and the allocation does not fail
// politely: it takes the process and the session with it.
func TestAnOverLargeRasterIsRefused(t *testing.T) {
	huge := func() *coverage.Raster {
		return &coverage.Raster{South: 56, North: 56.4, West: -3.6, East: -3.0,
			Width: 50000, Height: 50000, FreqMHz: 869.618}
	}

	r := huge()
	err := coverage.Compute(mast(), flat{100}, r, mastOpts())
	if err == nil {
		t.Fatal("Compute attempted a 50000 by 50000 raster")
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Errorf("the refusal does not name a limit: %v", err)
	}
	if r.Cells != nil {
		t.Errorf("cells were allocated anyway: %d", len(r.Cells))
	}

	if _, err := coverage.BestServer(levelGrid(8, 8), []coverage.Endpoint{mast()},
		huge(), mastOpts(), nil, nil); err == nil {
		t.Error("BestServer attempted a 50000 by 50000 raster")
	}
	if _, err := coverage.Combine([]*coverage.Raster{huge()}); err == nil {
		t.Error("Combine attempted a 50000 by 50000 raster")
	}
	if _, err := coverage.NewFold(56, 56.4, -3.6, -3.0, 50000, 50000, 869.618); err == nil {
		t.Error("NewFold attempted a 50000 by 50000 grid")
	}
}

// The ceiling is a ceiling and not a fence around the useful sizes: a grid on
// the map's own longest edge still has to go through, and one cell past the
// ceiling must not. Asserted at the edge rather than at the square, because
// allocating the square to prove it is allowed costs a gigabyte to learn what
// one comparison already says.
func TestTheCeilingSitsWhereTheWorkbenchStops(t *testing.T) {
	f, err := coverage.NewFold(56, 56.4, -3.6, -3.0, 4096, 16, 869.618)
	if err != nil {
		t.Fatalf("a grid on the map's own longest edge was refused: %v", err)
	}
	if f == nil {
		t.Fatal("no fold and no error")
	}
	if _, err := coverage.NewFold(56, 56.4, -3.6, -3.0, 4096, 4097, 869.618); err == nil {
		t.Error("a grid past the ceiling was accepted")
	}
}

// An empty raster is refused rather than silently producing nothing, and the
// paths that size arrays from the same two numbers refuse it identically.
func TestAnEmptyRasterIsRefusedEverywhere(t *testing.T) {
	for _, size := range [][2]int{{0, 8}, {8, 0}, {-4, 8}} {
		r := &coverage.Raster{South: 56, North: 56.4, West: -3.6, East: -3.0,
			Width: size[0], Height: size[1], FreqMHz: 869.618}
		if err := coverage.Compute(mast(), flat{100}, r, mastOpts()); err == nil {
			t.Errorf("Compute accepted a %dx%d raster", size[0], size[1])
		}
		if _, err := coverage.BestServer(levelGrid(8, 8), []coverage.Endpoint{mast()},
			r, mastOpts(), nil, nil); err == nil {
			t.Errorf("BestServer accepted a %dx%d raster", size[0], size[1])
		}
		if _, err := coverage.Combine([]*coverage.Raster{r}); err == nil {
			t.Errorf("Combine accepted a %dx%d raster", size[0], size[1])
		}
		if _, err := coverage.NewFold(56, 56.4, -3.6, -3.0,
			size[0], size[1], 869.618); err == nil {
			t.Errorf("NewFold accepted a %dx%d grid", size[0], size[1])
		}
	}
}

// No stations is a real question - a scenario before anything is placed - and
// the answer is "nothing is served", said in every field, rather than a panic
// or a cell claiming a server it has not got.
func TestNoStationsCoversNothing(t *testing.T) {
	r := &coverage.Raster{South: 56, North: 56.4, West: -3.6, East: -3.0,
		Width: 8, Height: 6, FreqMHz: 869.618}
	c, err := coverage.BestServer(levelGrid(16, 16), nil, r, mastOpts(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := range c.Cells {
		if c.BestNode[i] != -1 || c.ServingCount[i] != 0 {
			t.Fatalf("cell %d has a server with no stations: node %d, serving %d",
				i, c.BestNode[i], c.ServingCount[i])
		}
		if !math.IsNaN(c.BestMarginDB[i]) {
			t.Fatalf("cell %d has a margin of %.1f dB with no stations",
				i, c.BestMarginDB[i])
		}
	}
	if gaps, known := c.GapCells(); known != 0 || gaps != 0 {
		t.Fatalf("an empty network counted %d gaps over %d known cells", gaps, known)
	}
}

// A region of zero area is a mistyped box, not a coverage question. Every cell
// sits on the station, and the arithmetic still has to produce numbers rather
// than infinities somebody would go on to colour a map with.
func TestAZeroAreaRegionStaysFinite(t *testing.T) {
	r := &coverage.Raster{South: 56.2, North: 56.2, West: -3.3, East: -3.3,
		Width: 4, Height: 4, FreqMHz: 869.618}
	if err := coverage.Compute(mast(), flat{100}, r, mastOpts()); err != nil {
		t.Fatal(err)
	}
	for i, cell := range r.Cells {
		for _, v := range []float64{cell.OutboundMarginDB, cell.InboundMarginDB,
			cell.PathLossDB, cell.PositionSlackDB} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("cell %d of a zero-area region: %+v", i, cell)
			}
		}
	}
}
