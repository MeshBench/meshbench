package comp

import (
	"image"
	"math"

	"gioui.org/f32"
	"gioui.org/layout"
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
	sz image.Point, s *state.Snapshot) int {

	if len(s.Links) == 0 {
		return m.drawProximityLinks(t, gtx, pts, sz)
	}
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
	drawn := 0
	for _, b := range bands {
		var path clip.Path
		path.Begin(gtx.Ops)
		n := 0
		for _, l := range s.Links {
			if !l.Known || l.MarginDB < b.lo || l.MarginDB >= b.hi {
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
	return drawn
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
