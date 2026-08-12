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
	// Tiles is the basemap. Nil draws no basemap, which is what an offline
	// first run looks like, and the map still works.
	Tiles *Tiles
	// Zoom and Centre are the camera. Kept here rather than in state because
	// where somebody is looking is a property of the view, not of the world.
	Zoom          float64
	CentreLat     float64
	CentreLon     float64
	initialised   bool
	links         linkCache
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

	// The basemap first, under everything. Only cached tiles are drawn: a
	// redraw that waits on the network is a window that stops painting.
	if m.Tiles != nil {
		m.Tiles.Draw(gtx, sz, m.CentreLat, m.CentreLon, m.Zoom)
	}

	pts := m.project(s, sz)

	// Links, batched. One path, one fill, however many links.
	//
	// Which pairs are linked comes from the cache; where they are on screen
	// does not, so the cull stays here where the camera is known.
	var lp clip.Path
	lp.Begin(gtx.Ops)
	links := 0
	for _, pr := range m.links.get(pts) {
		a, b := pts[pr[0]], pts[pr[1]]
		if offscreen(a, sz) && offscreen(b, sz) {
			continue
		}
		segment(&lp, f32.Pt(a.x, a.y), f32.Pt(b.x, b.y), 1)
		links++
	}
	if links > 0 {
		paint.FillShape(gtx.Ops, theme.Alpha(t.P.Accent, 0.22),
			clip.Outline{Path: lp.End()}.Op())
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
			dot(&np, f32.Pt(p.x, p.y), 4)
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

func offscreen(p projected, sz image.Point) bool {
	const m = 40
	return p.x < -m || p.y < -m || p.x > float32(sz.X)+m || p.y > float32(sz.Y)+m
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
