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

	"github.com/MeshBench/meshbench/internal/coverage"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/linkbudget"
	"github.com/MeshBench/meshbench/internal/scenario"
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
	c, _, err := s.coverageWithRaster(ctx, n, spanKm)
	return c, err
}

func (s *Sim) coverageWithRaster(ctx context.Context, n scenario.Node, spanKm float64) (
	*state.Coverage, *coverage.Raster, error) {

	// A square of spanKm each way, in degrees.
	dLat := spanKm / 111.32
	dLon := spanKm / (111.32 * math.Cos(n.Position.Lat*math.Pi/180))
	r, err := s.rasterOnBox(ctx, n,
		n.Position.Lat-dLat, n.Position.Lat+dLat,
		n.Position.Lon-dLon, n.Position.Lon+dLon, covGrid, covGrid)
	if err != nil {
		return nil, nil, err
	}
	return paintCoverage(r, n.Name), r, nil
}

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

// rampFor is the legend: a continuous run from orange at the floor to
// green at 20 dB, HopReach's readability decision adopted whole - bands
// made a smooth physical quantity look like four verdicts, and the eye
// reads a gradient's shape where it only counts a band's edges. Above
// 20 dB stays the same green: more margin than that is not a distinction
// anybody acts on. The alpha is constant; the operator's opacity slider
// owns visibility now.
func rampFor(marginDB float64) color.RGBA {
	t := marginDB / 20
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	lerp := func(a, b float64) uint8 { return uint8(a + t*(b-a)) }
	return color.RGBA{
		R: lerp(230, 40), G: lerp(140, 190), B: lerp(50, 120), A: 150,
	}
}
