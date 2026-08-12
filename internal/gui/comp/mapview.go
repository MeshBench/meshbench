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
	Zoom        float64
	CentreLat   float64
	CentreLon   float64
	initialised bool
	links       linkCache
	cam         camera
	cut         float64
	cutSeq      uint64
	covOp       paint.ImageOp
	covFor      string
	labels      labeller
	sizes       labelSizer
	// Layers is what is drawn. Exported so a window, a menu or a script can
	// set it without reaching through the map.
	Layers Layers

	// OnSelect is called when the pointer changes the selection. Additive is
	// a shift-click or a shift-drag, which adds rather than replaces.
	OnSelect func(names []string, additive bool)
	// OnLayerOn is called when a layer is switched on, by its name. A layer
	// that needs computing - coverage - is asked for here rather than done
	// here.
	OnLayerOn func(layer string)
	// OnMove is called while a node is being dragged, every frame it moves,
	// so the rest of the interface follows the drag rather than jumping at
	// the end of it.
	OnMove func(name string, lat, lon float64)
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
	m.Layers.defaults()

	// The basemap first, under everything. Only cached tiles are drawn: a
	// redraw that waits on the network is a window that stops painting.
	pts := m.project(s, sz)
	m.handle(gtx, sz, pts)
	// The camera may have moved, so where things are on screen is recomputed
	// rather than reused: a frame that draws the old positions is a frame of
	// visible lag on every pan.
	if m.cam.drag == dragPan || m.cam.drag == dragNode {
		pts = m.project(s, sz)
	}

	// The basemap first, under everything. Only cached tiles are drawn: a
	// redraw that waits on the network is a window that stops painting.
	drawn, want := 0, 0
	if m.Tiles != nil && m.Layers.Basemap {
		drawn, want = m.Tiles.Draw(gtx, sz, m.CentreLat, m.CentreLon, m.Zoom)
	}

	// Coverage under everything but the basemap: it is the ground a network
	// sits on, and drawn over the links it would hide what it explains.
	if m.Layers.Coverage {
		m.drawCoverage(t, gtx, sz, s)
	}

	// The study boundaries, under the network.
	if m.Layers.Boundaries {
		m.drawAreas(t, gtx, sz, s)
	}

	// Links, weighted by the margin the engine measured. See mapworld.go.
	shownLinks, totalLinks := 0, 0
	if m.Layers.Links {
		shownLinks, totalLinks = m.drawLinks(t, gtx, pts, sz, s)
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
		if !m.Layers.Nodes {
			break
		}
		var np clip.Path
		np.Begin(gtx.Ops)
		for _, p := range list {
			dot(&np, f32.Pt(p.x, p.y), 4)
		}
		paint.FillShape(gtx.Ops, t.NodeColour(k), clip.Outline{Path: np.End()}.Op())
	}

	// Traffic over topology: a trail is what is happening now, and the
	// topology is what is always true.
	if m.Layers.Traffic {
		m.drawTrails(t, gtx, pts, sz, s)
	}

	// Selection and hover, drawn over the nodes as rings so that colour is
	// never the only thing carrying the state (11.8).
	m.rings(t, gtx, pts, sz)

	// Labels, placed so they do not overlap. See maplabels.go for why greedy
	// and why stable.
	spots := map[int]image.Point{}
	if m.Layers.Labels {
		spots = m.labels.place(pts, sz, m.cam.hover,
			func(i int) image.Point { return m.sizes.measure(gtx, t, pts[i].n.Name) })
	}
	for i, at := range spots {
		col := t.P.Dim
		if pts[i].n.Selected || i == m.cam.hover {
			col = t.P.Ink
		}
		off := op.Offset(at).Push(gtx.Ops)
		Text(t, t.Sz.Caption, col, pts[i].n.Name)(unbounded(gtx))
		off.Pop()
	}

	// The selection box, while one is being dragged.
	if m.cam.drag == dragBox && m.cam.moved {
		r := boxOf(m.cam.from, m.cam.boxTo)
		paint.FillShape(gtx.Ops, theme.Alpha(t.P.Accent, 0.12), clip.Rect(r).Op())
		off := op.Offset(r.Min).Push(gtx.Ops)
		Border(gtx, r.Size(), 0, 1, theme.Alpha(t.P.Accent, 0.8))
		off.Pop()
	}

	m.measureReadout(t, gtx, sz)
	m.scaleBar(t, gtx, sz, mapNote(s, shownLinks, totalLinks,
		basemapNote(drawn, want, m.Tiles != nil && m.Layers.Basemap)))
	m.layerPanel(t, gtx, sz)
	if m.Layers.Coverage {
		m.coverageLegend(t, gtx, sz, s)
	}

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

// rings draws the selection and hover states over the node dots.
//
// A ring rather than a different fill colour, because the fill already carries
// the node's kind and one channel cannot carry two things. It is also the only
// state visible to somebody who cannot separate the kind colours.
//
// Two passes rather than two paths built at once: clip.Path.Begin starts a
// macro, and two open macros cannot interleave.
func (m *MapView) rings(t *theme.Theme, gtx layout.Context, pts []projected, sz image.Point) {
	m.ringPass(gtx, pts, sz, theme.Alpha(t.P.Ink, 0.5), 9, 1,
		func(i int, p projected) bool { return i == m.cam.hover })
	m.ringPass(gtx, pts, sz, t.P.Selected, 7, 1.5,
		func(i int, p projected) bool { return p.n.Selected })
}

// ringPass draws one ring around every node the predicate accepts, as a single
// filled path.
func (m *MapView) ringPass(gtx layout.Context, pts []projected, sz image.Point,
	col color.NRGBA, r, w float32, want func(int, projected) bool) {

	var path clip.Path
	path.Begin(gtx.Ops)
	n := 0
	for i, p := range pts {
		if offscreen(p, sz) || !want(i, p) {
			continue
		}
		ring(&path, f32.Pt(p.x, p.y), r, w)
		n++
	}
	spec := path.End()
	if n == 0 {
		return
	}
	paint.FillShape(gtx.Ops, col, clip.Outline{Path: spec}.Op())
}

// ring is an annulus: an octagon outside an octagon wound the other way, so
// the non-zero fill leaves the middle empty and the node shows through.
func ring(p *clip.Path, c f32.Point, r, w float32) {
	dot(p, c, r)
	dotReversed(p, c, r-w)
}
