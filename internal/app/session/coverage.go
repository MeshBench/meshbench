// Coverage from one node, as a raster the map can draw.
//
// The computation is the same internal/coverage the planning tools use, with
// the same remote assumption - a person holding a handheld at 1.5 m - so a
// coverage picture here and a coverage answer there cannot disagree.
package session

import (
	"context"
	"image"
	"image/color"
	"math"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/study/coverage"
	"github.com/MeshBench/meshbench/internal/study/linkbudget"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// rasterOnBox is one node's raster over a caller-chosen box and grid - the
// shape the network-wide questions need, because rasters can only be combined
// when every node answered over the same ground.
func (s *Sim) rasterOnBox(_ context.Context, n scenario.Node,
	south, north, west, east float64, w, h int) (*coverage.Raster, error) {
	r := &coverage.Raster{
		South: south, North: north, West: west, East: east,
		Width: w, Height: h,
		Cells:   make([]coverage.Cell, w*h),
		FreqMHz: freqOf(n),
	}
	fixed := coverage.Endpoint{
		Name: n.Name, Lat: n.Position.Lat, Lon: n.Position.Lon,
		HeightAGLm: n.HeightAGLm, TxPowerDBm: n.TxPowerDBm,
		SensitivityDBm: linkbudget.SensitivityDBm(n),
		GainTowardsDBi: func(b, e float64) float64 { return n.Antenna.GainTowardsDBi(b, e) },
	}
	// The remote is a person with a handheld at 1.5 m, which is the assumption
	// the planning tools make. Stated here so that changing it is a decision
	// rather than a discovery.
	opts := coverage.Options{
		RemoteHeightAGLm: 1.5, RemoteTxPowerDBm: 20, RemoteGainDBi: 0,
		RemoteSensitivityDBm: linkbudget.SensitivityDBm(n), ProfileStepM: 120,
	}
	if err := coverage.Compute(fixed, s.terrain(), r, opts); err != nil {
		return nil, err
	}
	return r, nil
}

func freqOf(n scenario.Node) float64 {
	if n.Radio.CentreHz > 0 {
		return n.Radio.CentreHz / 1e6
	}
	return 869.618
}

// paintCoverage turns cells into pixels.
//
// Two-way first: a cell where the far end can be heard but cannot answer is
// drawn differently from one that works, because that asymmetry is the whole
// reason coverage is computed as a pair of margins rather than one.
func paintCoverage(r *coverage.Raster, name string) *state.Coverage {
	img := image.NewRGBA(image.Rect(0, 0, r.Width, r.Height))
	noData := 0
	for y := 0; y < r.Height; y++ {
		for x := 0; x < r.Width; x++ {
			c := r.At(x, y)
			var col color.RGBA
			switch {
			case c.NoData:
				noData++
				col = color.RGBA{} // transparent: ignorance is not a result
			case c.Workable():
				col = rampFor(math.Min(c.OutboundMarginDB, c.InboundMarginDB))
			case c.OneWay():
				// Heard but cannot answer. Its own amber, apart from the
				// ramp: asymmetry is a different fact, not a weaker margin.
				col = color.RGBA{R: 210, G: 120, B: 40, A: 150}
			default:
				col = color.RGBA{}
			}
			img.SetRGBA(x, y, col)
		}
	}
	return &state.Coverage{
		Node:  name,
		Image: img,
		South: r.South, North: r.North, West: r.West, East: r.East,
		NoDataCells: noData, Cells: r.Width * r.Height,
	}
}

// rampFor is the coverage overlay's colour, from the shared ramp so the
// picture and the legend beside it cannot drift.
//
// The alpha is this caller's: the overlay sits on a map and the operator's
// opacity slider owns how much of it shows.
func rampFor(marginDB float64) color.RGBA {
	c := coverage.Ramp(marginDB)
	c.A = 150
	return c
}
