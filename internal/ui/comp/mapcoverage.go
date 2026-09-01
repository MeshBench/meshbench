package comp

// The coverage overlay: a study's answer painted under the network, rather
// than the network itself. It sits apart from the world drawing because the
// two are asked for by different things - one is what the mesh is, the other
// is what a question about it returned - and because the raster path is
// shared with the hillshade, which is not a link, a node or a trail.

import (
	"fmt"
	"image"
	"math"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"image/color"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/study/coverage"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// drawCoverage paints the coverage raster under the network.
func (m *MapView) drawCoverage(t *theme.Theme, gtx layout.Context, sz image.Point,
	s *state.Snapshot) {
	m.drawRaster(gtx, sz, s.Coverage, &m.covOp, &m.covFor)
}

// drawRaster paints a geographic image into its place on the map.
//
// Shared by coverage and by the hillshade, because they are the same problem:
// an image with corners in degrees, drawn where those corners land, with the
// upload done once rather than per frame.
func (m *MapView) drawRaster(gtx layout.Context, sz image.Point, c *state.Coverage,
	cached *paint.ImageOp, cachedFor *string) {

	if c == nil || c.Image == nil {
		return
	}
	nw := m.projectPoint(state.Point{Lat: c.North, Lon: c.West}, sz)
	se := m.projectPoint(state.Point{Lat: c.South, Lon: c.East}, sz)
	w, h := se.X-nw.X, se.Y-nw.Y
	if w < 1 || h < 1 {
		return
	}
	if *cachedFor != c.Node || cached.Size().X == 0 {
		*cached = paint.NewImageOp(c.Image)
		// Nearest, not linear: zoomed in, a cell is a fact with edges, and
		// linear filtering smears it into a gradient that looks like data.
		cached.Filter = paint.FilterNearest
		*cachedFor = c.Node
	}
	off := op.Offset(image.Pt(int(nw.X), int(nw.Y))).Push(gtx.Ops)
	cl := clip.Rect{Max: image.Pt(int(w)+1, int(h)+1)}.Push(gtx.Ops)
	b := c.Image.Bounds()
	sc := op.Affine(f32Scale(w/float32(b.Dx()), h/float32(b.Dy()))).Push(gtx.Ops)
	cached.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	sc.Pop()
	cl.Pop()
	off.Pop()
}

// coverageLegend says what the colours mean, in dB.
//
// A coverage picture without a legend is a mood. The no-data count is on it
// too, because a raster computed with no elevation is a statement about the
// tile cache that looks exactly like a statement about radio.
func (m *MapView) coverageLegend(t *theme.Theme, gtx layout.Context, sz image.Point,
	s *state.Snapshot) {

	c := s.Coverage
	if c == nil {
		return
	}
	pad := gtx.Dp(t.Sp.S)
	sw := gtx.Dp(12)
	rec := op.Record(gtx.Ops)
	var kids []layout.FlexChild
	kids = append(kids, layout.Rigid(SectionTitle(t, "coverage from "+c.Node)))
	// The ramp as it actually is: one continuous run, labelled at its ends,
	// drawn from the same arithmetic the raster is painted with.
	kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		w, h := gtx.Dp(180), gtx.Dp(10)
		for x := 0; x < w; x++ {
			tt := float64(x) / float64(w-1)
			c := coverage.Ramp(tt * 20)
			col := color.NRGBA{R: c.R, G: c.G, B: c.B, A: 230}
			paint.FillShape(gtx.Ops, col,
				clip.Rect{Min: image.Pt(x, 0), Max: image.Pt(x+1, h)}.Op())
		}
		return layout.Dimensions{Size: image.Pt(w, h+gtx.Dp(2))}
	}))
	kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{}.Layout(gtx,
			layout.Rigid(Text(t, t.Sz.Caption, t.P.Faint, "0 dB")),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(180) - gtx.Dp(60)
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, 0)}
			}),
			layout.Rigid(Text(t, t.Sz.Caption, t.P.Faint, "20 dB and over")),
		)
	}))
	kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, color.NRGBA{R: 210, G: 120, B: 40, A: 200},
					clip.Rect{Max: image.Pt(sw, sw)}.Op())
				return layout.Dimensions{Size: image.Pt(sw+pad, sw)}
			}),
			layout.Rigid(Text(t, t.Sz.Caption, t.P.Dim, "heard, cannot answer")),
		)
	}))
	// The cell size in metres, so nobody has to guess what one pixel means -
	// and the honest cap: detail beyond the DEM's ~30 m is interpolation.
	if c.Image != nil && c.Image.Bounds().Dx() > 0 {
		midLat := (c.South + c.North) / 2
		cellM := (c.East - c.West) * 111320 *
			math.Cos(midLat*math.Pi/180) / float64(c.Image.Bounds().Dx())
		kids = append(kids, layout.Rigid(Text(t, t.Sz.Caption, t.P.Faint,
			fmt.Sprintf("cell size ~%.0f m", cellM))))
	}
	if c.NoDataCells > 0 {
		kids = append(kids, layout.Rigid(Text(t, t.Sz.Caption, t.P.Warn,
			fmt.Sprintf("%d of %d cells had no elevation data",
				c.NoDataCells, c.Cells))))
	}
	inner := gtx
	inner.Constraints.Min = image.Point{}
	inner.Constraints.Max = image.Pt(gtx.Dp(320), sz.Y)
	dims := layout.Flex{Axis: layout.Vertical}.Layout(inner, kids...)
	content := rec.Stop()

	box := image.Pt(dims.Size.X+pad*2, dims.Size.Y+pad*2)
	at := image.Pt(gtx.Dp(t.Sp.M), sz.Y-box.Y-gtx.Dp(t.Sp.XL)*2)
	off := op.Offset(at).Push(gtx.Ops)
	defer off.Pop()
	paint.FillShape(gtx.Ops, theme.Alpha(t.P.Panel, 0.9), clip.Rect{Max: box}.Op())
	Border(gtx, box, 2, 1, t.P.Rule)
	in := op.Offset(image.Pt(pad, pad)).Push(gtx.Ops)
	content.Add(gtx.Ops)
	in.Pop()
}
