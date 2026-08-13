package comp

import (
	"time"

	"image"
	"image/color"
	"math"
	"strings"

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
	Zoom float64
	// FitNext asks the next frame to frame every node. Set from anywhere;
	// cleared here once honoured.
	FitNext bool
	// Filter dims every node whose name or kind does not contain it. Dims
	// rather than hides, because a node that vanishes from a map looks like
	// a node that is not in the scenario at all.
	Filter string
	// Tool is what a click does: select, move, place, link or measure.
	Tool string
	// linkFrom is the first end of a link being made with the link tool. A
	// link takes two clicks, so the first one has to be remembered somewhere.
	linkFrom    string
	CentreLat   float64
	CentreLon   float64
	initialised bool
	links       linkCache
	cam         camera
	cut         float64
	cutSeq      uint64
	covOp       paint.ImageOp
	covFor      string
	shadeOp     paint.ImageOp
	shadeFor    string
	menu        mapMenu
	// ViewBox reports the visible bounds after each frame, so whatever wants
	// to compute something for this view can ask for the right area.
	ViewBox func(south, north, west, east float64)
	// OnSize reports the map's pixel size each frame, so an action taken from
	// a menu can frame the network without guessing how big the map is.
	OnSize func(image.Point)
	labels labeller
	sizes  labelSizer
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
	// OnBasemap is called when the basemap row at the top of the layer panel
	// is pressed. The choosing happens in the shell's overlay, like every
	// other pick-from-a-list.
	OnBasemap func()
	// OnMenu is called when a context menu entry is chosen: the action, the
	// node it was opened on if any, and where on the ground it was opened.
	OnMenu func(action, node string, lat, lon float64)
	// OnMove is called while a node is being dragged, every frame it moves,
	// so the rest of the interface follows the drag rather than jumping at
	// the end of it.
	OnMove func(name string, lat, lon float64)
	// OnPlace is called when the place tool is used, with where on the ground
	// it was clicked. What kind of node to put there is the workbench's
	// decision, not the map's.
	OnPlace func(lat, lon float64)
	// OnNodeOpen is called when a node is double-clicked, which is the
	// gesture that means "show me this one" everywhere else.
	OnNodeOpen func(name string)
	// lastClick remembers the previous click for the double-click test.
	lastClickName string
	lastClickAt   time.Duration
	// OnLinkPair is called when the link tool has been given two nodes. What
	// to do with the pair - select them and break the link into its terms -
	// belongs to whoever knows what a link budget is.
	OnLinkPair func(a, b string)
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
	if !m.initialised || m.FitNext {
		// FitNext is how something outside the frame loop asks for a fit:
		// framing needs the widget's size, which only exists here.
		m.fit(s, sz)
		m.initialised, m.FitNext = true, false
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

	// Relief over the basemap and under everything else.
	if m.Layers.Terrain && s.Shade != nil {
		m.drawRaster(gtx, sz, s.Shade, &m.shadeOp, &m.shadeFor)
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
	// per node, and split by whether the filter matches. Two passes, because
	// one path can only have one colour.
	byKind := map[theme.NodeKind][]projected{}
	dimKind := map[theme.NodeKind][]projected{}
	filterWant := strings.ToLower(strings.TrimSpace(m.Filter))
	for _, p := range pts {
		if offscreen(p, sz) {
			continue
		}
		k := kindOf(p.n.Kind)
		if int(k) < len(m.Layers.HideKind) && m.Layers.HideKind[k] {
			continue
		}
		if filterWant != "" && !nodeMatches(p.n, filterWant) {
			dimKind[k] = append(dimKind[k], p)
			continue
		}
		byKind[k] = append(byKind[k], p)
	}
	if m.Layers.Nodes {
		// The ones that do not match are drawn first and faint, so they stay
		// on the map. A node that vanishes reads as a node that is not in the
		// scenario, which is a different and much more alarming thing.
		for k, list := range dimKind {
			var np clip.Path
			np.Begin(gtx.Ops)
			for _, p := range list {
				dot(&np, f32.Pt(p.x, p.y), 3)
			}
			paint.FillShape(gtx.Ops, theme.Alpha(t.NodeColour(k), 0.22),
				clip.Outline{Path: np.End()}.Op())
		}
		// A dark ring under every marker first, one path for all of them, so
		// a dot reads against a dark hillshade and a light street map without
		// changing colour with the layer.
		var ring clip.Path
		ring.Begin(gtx.Ops)
		for _, list := range byKind {
			for _, p := range list {
				dot(&ring, f32.Pt(p.x, p.y), 6)
			}
		}
		paint.FillShape(gtx.Ops, t.P.MapPlate, clip.Outline{Path: ring.End()}.Op())
		for k, list := range byKind {
			var np clip.Path
			np.Begin(gtx.Ops)
			for _, p := range list {
				dot(&np, f32.Pt(p.x, p.y), 5)
			}
			paint.FillShape(gtx.Ops, t.NodeColour(k), clip.Outline{Path: np.End()}.Op())
		}
	}

	// What the selection tells you, over the topology and under the traffic.
	if m.Layers.Regions {
		m.drawRegions(t, gtx, pts, sz)
	}
	m.drawNeighbours(t, gtx, pts, sz, s)
	if m.Layers.Pattern {
		m.drawPattern(t, gtx, pts, sz)
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
		// The old workbench's recipe, kept because it is proven: near-white
		// ink on a dark plate, whatever the theme and whatever the basemap.
		// A label on the map contends with the map's own colours - white text
		// was invisible over the light street map, which is exactly where the
		// place names it competes with already are.
		col := theme.Alpha(t.P.MapInk, 0.85)
		if pts[i].n.Selected || i == m.cam.hover {
			col = t.P.MapInk
		}
		off := op.Offset(at).Push(gtx.Ops)
		if sz := m.sizes.measure(gtx, t, pts[i].n.Name); sz.X > 0 {
			pad := gtx.Dp(3)
			RoundRect(gtx, image.Pt(sz.X+pad*2, sz.Y), 3, t.P.MapPlate)
			in := op.Offset(image.Pt(pad, 0)).Push(gtx.Ops)
			Text(t, t.Sz.Caption, col, pts[i].n.Name)(unbounded(gtx))
			in.Pop()
		}
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
	attrib := ""
	if m.Tiles != nil {
		attrib = m.Tiles.Layer.Attribution
	}
	m.scaleBar(t, gtx, sz, mapNote(s, shownLinks, totalLinks,
		basemapNote(drawn, want, m.Tiles != nil && m.Layers.Basemap, attrib)))
	m.layerPanel(t, gtx, sz, s)
	if m.Layers.Coverage {
		m.coverageLegend(t, gtx, sz, s)
	}
	m.layoutMenu(t, gtx, sz)

	if m.OnSize != nil {
		m.OnSize(sz)
	}
	if m.ViewBox != nil {
		south, west := m.unproject(f32.Pt(0, float32(sz.Y)), sz)
		north, east := m.unproject(f32.Pt(float32(sz.X), 0), sz)
		m.ViewBox(south, north, west, east)
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

// Fit frames every node with a margin (10.9).
func (m *MapView) Fit(s *state.Snapshot, sz image.Point) {
	if s == nil || len(s.Nodes) == 0 {
		return
	}
	m.fit(s, sz)
}

// FocusOn centres the camera on a named node without changing the zoom, which
// is what "centre on this" means: the same view, somewhere else.
func (m *MapView) FocusOn(s *state.Snapshot, name string) bool {
	if s == nil {
		return false
	}
	for i := range s.Nodes {
		if s.Nodes[i].Name == name {
			m.CentreOn(s.Nodes[i].Lat, s.Nodes[i].Lon)
			return true
		}
	}
	return false
}

// nodeMatches is the map filter: name or kind, case-insensitively.
func nodeMatches(n *state.Node, want string) bool {
	return strings.Contains(strings.ToLower(n.Name), want) ||
		strings.Contains(strings.ToLower(n.Kind), want)
}
