package comp

import (
	"fmt"
	"image"
	"math"
	"sort"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"image/color"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// The world under the network: boundaries, the study margin, and links drawn
// by how much margin they actually have.

// links draws every link the store computed, weighted by margin.
//
// Nothing here decides whether a link exists: that came from the engine's own
// path loss, through the same link budget the budget panel uses, so the map
// and the panel cannot disagree.
func (m *MapView) drawLinks(t *theme.Theme, gtx layout.Context, pts []projected,
	sz image.Point, s *state.Snapshot) (drawn, total int) {

	if len(s.Links) == 0 {
		return m.drawProximityLinks(t, gtx, pts, sz), 0
	}
	cut := m.linkCut(s)
	// One path per band, so a frame is three fills rather than one per link.
	bands := []struct {
		lo, hi float64
		w      float32
		a      float32
		c      func(p theme.Palette) [3]uint8
	}{
		{6, math.Inf(1), 1.4, 0.55, func(p theme.Palette) [3]uint8 {
			return [3]uint8{p.Good.R, p.Good.G, p.Good.B}
		}},
		{0, 6, 1.0, 0.45, func(p theme.Palette) [3]uint8 {
			return [3]uint8{p.Warn.R, p.Warn.G, p.Warn.B}
		}},
		{math.Inf(-1), 0, 0.8, 0.30, func(p theme.Palette) [3]uint8 {
			return [3]uint8{p.Bad.R, p.Bad.G, p.Bad.B}
		}},
	}
	// The band that does not close is off by default. On this fixture it is
	// 11,000 links against 1,200 that do, and drawing it swamps every link
	// anybody wants to see. It is a switch rather than a deletion, because
	// "which pairs nearly hear each other" is a real question.
	if !m.Layers.WeakLinks {
		bands = bands[:2]
	}
	for _, b := range bands {
		var path clip.Path
		path.Begin(gtx.Ops)
		n := 0
		for _, l := range s.Links {
			if !l.Known || l.MarginDB < b.lo || l.MarginDB >= b.hi {
				continue
			}
			if l.MarginDB < cut {
				continue
			}
			if l.A >= len(pts) || l.B >= len(pts) {
				continue
			}
			a, c := pts[l.A], pts[l.B]
			if offscreen(a, sz) && offscreen(c, sz) {
				continue
			}
			dx, dy := a.x-c.x, a.y-c.y
			if dx*dx+dy*dy < 64 {
				continue
			}
			segment(&path, f32.Pt(a.x, a.y), f32.Pt(c.x, c.y), b.w)
			n++
		}
		spec := path.End()
		if n == 0 {
			continue
		}
		rgb := b.c(t.P)
		col := t.P.Accent
		col.R, col.G, col.B = rgb[0], rgb[1], rgb[2]
		paint.FillShape(gtx.Ops, theme.Alpha(col, float32(b.a)), clip.Outline{Path: spec}.Op())
		drawn += n
	}
	return drawn, len(s.Links)
}

// maxDrawnLinks is how many the map will draw before it starts taking the
// strongest.
//
// Not tidiness: with no terrain tiles cached the engine has no profile to cut
// a path against, so links close at distances they would not over real ground
// and the 311 node fixture produced 12,924 of them. Drawing all of those took
// long enough per frame that the compositor marked the window as not
// responding. The cap is stated on the map rather than applied quietly,
// because a map showing a tenth of the links without saying so is worse than a
// slow one.
const maxDrawnLinks = 2500

// linkCut is the margin below which links are not drawn, chosen so that at
// most maxDrawnLinks are.
//
// Recomputed only when the snapshot changes, because sorting twelve thousand
// margins every frame is the problem it was meant to solve.
func (m *MapView) linkCut(s *state.Snapshot) float64 {
	if s.Seq == m.cutSeq {
		return m.cut
	}
	m.cutSeq = s.Seq
	m.cut = math.Inf(-1)
	if len(s.Links) <= maxDrawnLinks {
		return m.cut
	}
	margins := make([]float64, 0, len(s.Links))
	for _, l := range s.Links {
		if l.Known {
			margins = append(margins, l.MarginDB)
		}
	}
	sort.Float64s(margins)
	if n := len(margins) - maxDrawnLinks; n > 0 {
		m.cut = margins[n]
	}
	return m.cut
}

// drawProximityLinks is the fallback for a network nothing has computed
// margins for yet - a fixture still loading, or a test.
//
// It says so by drawing thinner and fainter than any real band, because a
// guess that looks like a measurement is the worst of both.
func (m *MapView) drawProximityLinks(t *theme.Theme, gtx layout.Context,
	pts []projected, sz image.Point) int {

	var lp clip.Path
	lp.Begin(gtx.Ops)
	n := 0
	for _, pr := range m.links.get(pts) {
		a, b := pts[pr[0]], pts[pr[1]]
		if offscreen(a, sz) && offscreen(b, sz) {
			continue
		}
		dx, dy := a.x-b.x, a.y-b.y
		if dx*dx+dy*dy < 64 {
			continue
		}
		segment(&lp, f32.Pt(a.x, a.y), f32.Pt(b.x, b.y), 1)
		n++
	}
	spec := lp.End()
	if n > 0 {
		paint.FillShape(gtx.Ops, theme.Alpha(t.P.Faint, 0.35),
			clip.Outline{Path: spec}.Op())
	}
	return n
}

// drawAreas outlines the study boundaries and the margin around them.
//
// The margin is drawn because it is a real part of the study: nodes outside
// the boundary but inside the margin are simulated, and somebody looking at
// the map should be able to see which those are rather than wonder why a
// repeater in Cumbria is in a Scottish study.
func (m *MapView) drawAreas(t *theme.Theme, gtx layout.Context, sz image.Point,
	s *state.Snapshot) {

	if len(s.Areas) == 0 {
		return
	}
	for _, a := range s.Areas {
		m.ringPath(t, gtx, sz, a.Rings, theme.Alpha(t.P.Accent, 0.5), 1.5)
		m.ringPath(t, gtx, sz, a.Holes, theme.Alpha(t.P.Accent, 0.25), 1)
	}
}

// ringPath strokes a set of rings as one path.
func (m *MapView) ringPath(t *theme.Theme, gtx layout.Context, sz image.Point,
	rings [][]state.Point, col color.NRGBA, width float32) {

	var path clip.Path
	path.Begin(gtx.Ops)
	n := 0
	for _, outline := range rings {
		if len(outline) < 2 {
			continue
		}
		prev := m.projectPoint(outline[0], sz)
		for _, p := range outline[1:] {
			cur := m.projectPoint(p, sz)
			// Rings are dense - a national boundary is tens of thousands of
			// points - so drop anything that would move less than a pixel.
			dx, dy := cur.X-prev.X, cur.Y-prev.Y
			if dx*dx+dy*dy < 4 {
				continue
			}
			segment(&path, prev, cur, width)
			prev = cur
			n++
		}
	}
	spec := path.End()
	if n > 0 {
		paint.FillShape(gtx.Ops, col, clip.Outline{Path: spec}.Op())
	}
}

// projectPoint is project() for a single position.
func (m *MapView) projectPoint(p state.Point, sz image.Point) f32.Point {
	cos := math.Cos(m.CentreLat * math.Pi / 180)
	return f32.Pt(
		float32(float64(sz.X)/2+(p.Lon-m.CentreLon)*cos*m.Zoom),
		float32(float64(sz.Y)/2-(p.Lat-m.CentreLat)*m.Zoom),
	)
}

// trailWindowMs must match what fills Snapshot.Trails: the store decides how
// far back a trail is kept, and the map only decides how to fade it.
const trailWindowMs = 4000

// drawTrails fades recent transmissions out over the window they are kept for.
//
// Newest brightest, and drawn last so traffic sits over the topology rather
// than under it. A delivered hop is a line between two nodes; a transmission
// nobody received is a short stub in no particular direction, because it went
// nowhere and drawing it as reaching somewhere would be a lie about the run.
func (m *MapView) drawTrails(t *theme.Theme, gtx layout.Context, pts []projected,
	sz image.Point, s *state.Snapshot) {

	if len(s.Trails) == 0 {
		return
	}
	// Grouped into a few alpha steps, because one fill per trail is one draw
	// call per packet and a busy mesh has thousands.
	const steps = 4
	for step := 0; step < steps; step++ {
		newest := float64(step) / steps
		oldest := float64(step+1) / steps
		var path clip.Path
		path.Begin(gtx.Ops)
		n := 0
		for _, tr := range s.Trails {
			age := float64(s.NowMs-tr.AtMs) / trailWindowMs
			if age < newest || age >= oldest || age > 1 {
				continue
			}
			if tr.From < 0 || tr.From >= len(pts) {
				continue
			}
			a := pts[tr.From]
			if tr.To < 0 || tr.To >= len(pts) {
				// A transmission with no receiver: a stub upwards, which is
				// not a direction on the map and cannot be read as one.
				segment(&path, f32.Pt(a.x, a.y), f32.Pt(a.x, a.y-7), 2)
				n++
				continue
			}
			b := pts[tr.To]
			if offscreen(a, sz) && offscreen(b, sz) {
				continue
			}
			segment(&path, f32.Pt(a.x, a.y), f32.Pt(b.x, b.y), 2)
			n++
		}
		spec := path.End()
		if n == 0 {
			continue
		}
		// Ink rather than the selection colour: a trail has to read against
		// links that are already green and amber, and the colour left that is
		// not one of those is no colour at all.
		alpha := float32(0.9 * (1 - float64(step)/steps))
		paint.FillShape(gtx.Ops, theme.Alpha(t.P.Ink, alpha),
			clip.Outline{Path: spec}.Op())
	}
}

// drawCoverage paints the raster under the network, and its legend over it.
//
// Under, because coverage is the ground a network sits on: drawn over the
// links it would hide the thing it is meant to explain.
func (m *MapView) drawCoverage(t *theme.Theme, gtx layout.Context, sz image.Point,
	s *state.Snapshot) {

	c := s.Coverage
	if c == nil || c.Image == nil {
		return
	}
	nw := m.projectPoint(state.Point{Lat: c.North, Lon: c.West}, sz)
	se := m.projectPoint(state.Point{Lat: c.South, Lon: c.East}, sz)
	w, h := se.X-nw.X, se.Y-nw.Y
	if w < 1 || h < 1 {
		return
	}
	if m.covOp.Size().X == 0 || m.covFor != c.Node {
		m.covOp = paint.NewImageOp(c.Image)
		m.covFor = c.Node
	}
	off := op.Offset(image.Pt(int(nw.X), int(nw.Y))).Push(gtx.Ops)
	cl := clip.Rect{Max: image.Pt(int(w)+1, int(h)+1)}.Push(gtx.Ops)
	b := c.Image.Bounds()
	sc := op.Affine(f32Scale(w/float32(b.Dx()), h/float32(b.Dy()))).Push(gtx.Ops)
	m.covOp.Add(gtx.Ops)
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
	rows := []struct {
		col   color.NRGBA
		label string
	}{
		{color.NRGBA{R: 40, G: 170, B: 120, A: 200}, "20 dB and over"},
		{color.NRGBA{R: 90, G: 180, B: 100, A: 200}, "10 to 20 dB"},
		{color.NRGBA{R: 200, G: 180, B: 70, A: 200}, "3 to 10 dB"},
		{color.NRGBA{R: 210, G: 130, B: 60, A: 200}, "0 to 3 dB"},
		{color.NRGBA{R: 200, G: 120, B: 40, A: 140}, "heard, cannot answer"},
	}
	pad := gtx.Dp(t.Sp.S)
	sw := gtx.Dp(12)
	rec := op.Record(gtx.Ops)
	var kids []layout.FlexChild
	kids = append(kids, layout.Rigid(SectionTitle(t, "coverage from "+c.Node)))
	for _, r := range rows {
		r := r
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					paint.FillShape(gtx.Ops, r.col, clip.Rect{Max: image.Pt(sw, sw)}.Op())
					return layout.Dimensions{Size: image.Pt(sw+pad, sw)}
				}),
				layout.Rigid(Text(t, t.Sz.Caption, t.P.Dim, r.label)),
			)
		}))
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
