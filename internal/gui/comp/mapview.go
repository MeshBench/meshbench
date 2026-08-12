package comp

import (
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// MapView draws the network.
//
// Two things here are the frame-budget work the spike said would be needed:
// links are batched into one path rather than one draw call each, and anything
// outside the viewport is skipped before it becomes a draw op at all. The spike
// measured 24 fps naive, 35 batched; culling is the other half.
type MapView struct {
	// Zoom and Centre are the camera. Kept here rather than in state because
	// where somebody is looking is a property of the view, not of the world.
	Zoom          float64
	CentreLat     float64
	CentreLon     float64
	initialised   bool
	LabelEveryNth int
}

type projected struct {
	x, y float32
	n    *state.Node
}

// Layout draws the map for one snapshot.
func (m *MapView) Layout(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	sz := gtx.Constraints.Max
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: t.P.Sunk}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)

	if s == nil || len(s.Nodes) == 0 {
		return layout.Center.Layout(gtx,
			Text(t, t.Sz.Caption, t.P.Faint, "no network loaded"))
	}
	if !m.initialised {
		m.fit(s, sz)
		m.initialised = true
	}
	if m.LabelEveryNth == 0 {
		m.LabelEveryNth = 4
	}

	pts := m.project(s, sz)

	// Links, batched. One path, one fill, however many links.
	var lp clip.Path
	lp.Begin(gtx.Ops)
	links := 0
	for i := range pts {
		for j := i + 1; j < len(pts); j++ {
			if !near(pts[i].n, pts[j].n) {
				continue
			}
			// Cull: a segment with both ends off-screen cannot be visible.
			if offscreen(pts[i], sz) && offscreen(pts[j], sz) {
				continue
			}
			lp.MoveTo(f32.Pt(pts[i].x, pts[i].y))
			lp.LineTo(f32.Pt(pts[j].x, pts[j].y))
			links++
		}
	}
	if links > 0 {
		paint.FillShape(gtx.Ops, theme.Alpha(t.P.Accent, 0.22),
			clip.Stroke{Path: lp.End(), Width: 1}.Op())
	}

	// Nodes, grouped by kind so each kind is one filled path rather than one
	// per node.
	byKind := map[theme.NodeKind][]projected{}
	for _, p := range pts {
		if offscreen(p, sz) {
			continue
		}
		byKind[kindOf(p.n.Kind)] = append(byKind[kindOf(p.n.Kind)], p)
	}
	for k, list := range byKind {
		var np clip.Path
		np.Begin(gtx.Ops)
		for _, p := range list {
			circle(&np, f32.Pt(p.x, p.y), 4)
		}
		paint.FillShape(gtx.Ops, t.NodeColour(k), clip.Outline{Path: np.End()}.Op())
	}

	// Labels, thinned: at national scale every name is unreadable anyway, and
	// each one is a text shaping call.
	shown := 0
	for i, p := range pts {
		if offscreen(p, sz) {
			continue
		}
		if kindOf(p.n.Kind) == theme.SimpleRepeater && i%m.LabelEveryNth != 0 && !p.n.Selected {
			continue
		}
		col := t.P.Dim
		if p.n.Selected {
			col = t.P.Ink
		}
		off := op.Offset(image.Pt(int(p.x)+8, int(p.y)-7)).Push(gtx.Ops)
		Text(t, t.Sz.Caption, col, p.n.Name)(gtx)
		off.Pop()
		shown++
	}

	// Scale bar and attribution, bottom left, as the old map had.
	off := op.Offset(image.Pt(gtx.Dp(t.Sp.M), sz.Y-gtx.Dp(t.Sp.XL))).Push(gtx.Ops)
	Mono(t, t.Sz.Caption, t.P.Faint,
		"20 km    Elevation: AWS terrarium    (c) OpenStreetMap")(gtx)
	off.Pop()

	return layout.Dimensions{Size: sz}
}

func (m *MapView) fit(s *state.Snapshot, sz image.Point) {
	minLat, maxLat := 90.0, -90.0
	minLon, maxLon := 180.0, -180.0
	for _, n := range s.Nodes {
		if n.Lat == 0 && n.Lon == 0 {
			continue
		}
		minLat, maxLat = math.Min(minLat, n.Lat), math.Max(maxLat, n.Lat)
		minLon, maxLon = math.Min(minLon, n.Lon), math.Max(maxLon, n.Lon)
	}
	m.CentreLat, m.CentreLon = (minLat+maxLat)/2, (minLon+maxLon)/2
	cos := math.Cos(m.CentreLat * math.Pi / 180)
	spanX, spanY := (maxLon-minLon)*cos, maxLat-minLat
	if spanX <= 0 || spanY <= 0 {
		m.Zoom = 1000
		return
	}
	m.Zoom = math.Min(float64(sz.X-80)/spanX, float64(sz.Y-80)/spanY)
}

func (m *MapView) project(s *state.Snapshot, sz image.Point) []projected {
	cos := math.Cos(m.CentreLat * math.Pi / 180)
	out := make([]projected, 0, len(s.Nodes))
	for i := range s.Nodes {
		n := &s.Nodes[i]
		if n.Lat == 0 && n.Lon == 0 {
			continue
		}
		x := float64(sz.X)/2 + (n.Lon-m.CentreLon)*cos*m.Zoom
		y := float64(sz.Y)/2 - (n.Lat-m.CentreLat)*m.Zoom
		out = append(out, projected{x: float32(x), y: float32(y), n: n})
	}
	return out
}

// near is the same proximity rule the rest of the tool uses for a quick link
// estimate: close enough to hear on this band.
func near(a, b *state.Node) bool {
	const r = 6371.0
	dLat := (b.Lat - a.Lat) * math.Pi / 180
	dLon := (b.Lon - a.Lon) * math.Pi / 180
	la, lb := a.Lat*math.Pi/180, b.Lat*math.Pi/180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(la)*math.Cos(lb)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2*r*math.Asin(math.Sqrt(h)) < 18
}

func offscreen(p projected, sz image.Point) bool {
	const m = 40
	return p.x < -m || p.y < -m || p.x > float32(sz.X)+m || p.y > float32(sz.Y)+m
}

func circle(p *clip.Path, c f32.Point, r float32) {
	const k = 0.5522847498
	p.MoveTo(f32.Pt(c.X+r, c.Y))
	p.CubeTo(f32.Pt(c.X+r, c.Y+r*k), f32.Pt(c.X+r*k, c.Y+r), f32.Pt(c.X, c.Y+r))
	p.CubeTo(f32.Pt(c.X-r*k, c.Y+r), f32.Pt(c.X-r, c.Y+r*k), f32.Pt(c.X-r, c.Y))
	p.CubeTo(f32.Pt(c.X-r, c.Y-r*k), f32.Pt(c.X-r*k, c.Y-r), f32.Pt(c.X, c.Y-r))
	p.CubeTo(f32.Pt(c.X+r*k, c.Y-r), f32.Pt(c.X+r, c.Y-r*k), f32.Pt(c.X+r, c.Y))
	p.Close()
}

func kindOf(k string) theme.NodeKind {
	switch k {
	case "companion":
		return theme.Companion
	case "room-server":
		return theme.RoomServer
	case "sdr-observer":
		return theme.Observer
	case "emitter":
		return theme.Emitter
	case "advanced-repeater":
		return theme.AdvancedRepeater
	}
	return theme.SimpleRepeater
}

var _ = color.NRGBA{}
