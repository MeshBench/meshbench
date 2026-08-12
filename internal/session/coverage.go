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

	"github.com/A13xB0/meshcoresim/internal/coverage"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/linkbudget"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// covGrid is the raster resolution. 160 square is 25,600 cells, each one a
// terrain profile: seconds, not minutes, and fine enough that the edge of
// coverage is a shape rather than a staircase.
const covGrid = 160

// coverageFor computes the raster around one node and paints it.
//
// Returned as an image rather than as cells because the snapshot crosses into
// the renderer, and a renderer that has to know what a decibel is in order to
// draw a picture is a renderer that will eventually disagree with the panel
// that prints the number.
func (s *Sim) coverageFor(ctx context.Context, n scenario.Node, spanKm float64) (
	*state.Coverage, error) {

	// A square of spanKm each way, in degrees.
	dLat := spanKm / 111.32
	dLon := spanKm / (111.32 * math.Cos(n.Position.Lat*math.Pi/180))
	r := &coverage.Raster{
		South: n.Position.Lat - dLat, North: n.Position.Lat + dLat,
		West: n.Position.Lon - dLon, East: n.Position.Lon + dLon,
		Width: covGrid, Height: covGrid,
		Cells:   make([]coverage.Cell, covGrid*covGrid),
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
	return paintCoverage(r, n), nil
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
func paintCoverage(r *coverage.Raster, n scenario.Node) *state.Coverage {
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
				// Heard but cannot answer. Hatched by alpha rather than by a
				// pattern, which does not survive being scaled on a map.
				col = color.RGBA{R: 200, G: 120, B: 40, A: 90}
			default:
				col = color.RGBA{}
			}
			img.SetRGBA(x, y, col)
		}
	}
	return &state.Coverage{
		Node:  n.Name,
		Image: img,
		South: r.South, North: r.North, West: r.West, East: r.East,
		NoDataCells: noData, Cells: r.Width * r.Height,
	}
}

// rampFor is the legend: green comfortable, amber marginal, and nothing above
// 30 dB because more margin than that is not a distinction anybody acts on.
func rampFor(marginDB float64) color.RGBA {
	switch {
	case marginDB >= 20:
		return color.RGBA{R: 40, G: 170, B: 120, A: 120}
	case marginDB >= 10:
		return color.RGBA{R: 90, G: 180, B: 100, A: 110}
	case marginDB >= 3:
		return color.RGBA{R: 200, G: 180, B: 70, A: 105}
	}
	return color.RGBA{R: 210, G: 130, B: 60, A: 100}
}
