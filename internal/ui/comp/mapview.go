package comp

import (
	"time"

	"image"
	"image/color"
	"strings"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"
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
	// OverlayTiles are the layers drawn above the coverage raster - roads,
	// then labels - so the ground's structure and its names come through
	// the picture instead of drowning under it. Empty where the base has
	// everything baked in.
	OverlayTiles []*Tiles
	// Zoom and Centre are the camera. Kept here rather than in state because
	// where somebody is looking is a property of the view, not of the world.
	Zoom float64
	// zoomTarget is where the wheel has asked the zoom to end up, and
	// zoomAnchor the screen point it is scaling about. The camera closes on
	// the target over a few frames rather than arriving at once, which is
	// what stops a wheel notch reading as a jump.
	zoomTarget float64
	zoomAnchor f32.Point
	// anchorLat/anchorLon are the ground position under zoomAnchor when the
	// gesture began, held for the whole glide so every frame can put it back
	// exactly rather than nudging toward it.
	anchorLat, anchorLon float64
	zooming              bool
	lastFrameAt          time.Time
	// FitNext asks the next frame to frame every node. Set from anywhere;
	// cleared here once honoured.
	FitNext bool
	// FitLoaded is the softer request a network arriving makes: frame it,
	// unless the camera was pinned by a flag. Somebody capturing a particular
	// place asked for that place, and a fixture finishing its load a few
	// seconds later is not a reason to overrule them - while a network opened
	// onto a camera nobody aimed is a blank map that reads as a failed open.
	FitLoaded bool
	// pinned is that deliberate placement, set by StartAt. An outright fit -
	// the menu entry, or the map.fit verb - is somebody asking, and ignores
	// it.
	pinned bool
	// Filter dims every node whose name or kind does not contain it. Dims
	// rather than hides, because a node that vanishes from a map looks like
	// a node that is not in the scenario at all.
	Filter string
	// Tool is what a click does: select, move, place, link or measure.
	Tool string
	// linkFrom is the first end of a link being made with the link tool. A
	// link takes two clicks, so the first one has to be remembered somewhere.
	linkFrom *LinkEnd
	// PinnedLink says a pair picked with the link tool is holding the Link
	// panel: ordinary selection still selects, but stops refreshing the
	// budget over the pinned pair. Cleared by CancelLink.
	PinnedLink  bool
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
	// BuildingsIn returns the footprints inside a lat/lon box, or nil when
	// no environment is loaded. Wired by the workbench so the map itself
	// stays data-blind.
	BuildingsIn func(south, west, north, east float64) []state.BuildingPoly
	bldCache    buildingsCache
	// CoverageOpacity is the raster's draw-time opacity, the slider beside
	// the layer panel. It starts below full - the ground and its names
	// belong in the picture the raster explains - and zero is a position
	// the operator can actually reach, meaning "raster off the picture".
	CoverageOpacity Slider
	// OnRasterView asks the session to raster exactly this viewport - the
	// borders someone is looking at, not the network's or the boundary's.
	OnRasterView  func(south, west, north, east float64, cells int)
	rasterViewBtn widget.Clickable
	lastSize      image.Point

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
	// OnDelete is called with every node the Delete key was pressed over -
	// the selection, since that is what the key acts on everywhere else.
	//
	// Separate from OnMenu, which carries one node: a box drag selects
	// several and asking the same question once per node is not a
	// confirmation, it is an obstacle. The menu entry routes here too, with
	// a slice of one, so the key and the menu cannot come to mean different
	// things.
	OnDelete func(names []string)
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
	// OnLinkPair is called when the link tool has been given two ends. What
	// to do with the pair - break the link into its terms - belongs to
	// whoever knows what a link budget is.
	OnLinkPair func(a, b LinkEnd)
	// OnLinkArmed says the first end has been picked, so a hint can tell the
	// operator what the next click means; OnLinkCancel says the tool's state
	// was abandoned, which is also the moment to release a pinned pair.
	OnLinkArmed  func(end LinkEnd)
	OnLinkCancel func()
}

// Layout draws the map for one snapshot.
func (m *MapView) Layout(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	sz := gtx.Constraints.Max
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: t.P.Sunk}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)

	if s == nil {
		return layout.Center.Layout(gtx,
			Text(t, t.Sz.Caption, t.P.Faint, "no network loaded"))
	}
	// A network with no nodes still gets a map.
	//
	// It used to get this line instead of one, which is a blank network
	// nobody can put a repeater on: the place tool needs ground to click on,
	// and the ground is the thing that was being withheld.
	empty := len(s.Nodes) == 0
	// Remembered for the callers outside the frame loop - the menu's
	// raster-this-view needs the viewport, and the viewport only exists
	// where the widget has a size.
	m.lastSize = sz
	if m.wantsFit() {
		// The request comes from outside the frame loop: framing needs the
		// widget's size, which only exists here.
		m.fit(s, sz)
		m.initialised, m.FitNext = true, false
	}
	// Cleared whether or not it was honoured: a request a pinned camera
	// declined is a request that has been answered, not one still waiting.
	m.FitLoaded = false
	m.Layers.defaults()

	// The basemap first, under everything. Only cached tiles are drawn: a
	// redraw that waits on the network is a window that stops painting.
	pts := m.project(s, sz)
	m.handle(gtx, sz, pts)

	// Advance the zoom glide, and ask for another frame while it is still
	// moving. Gio does not redraw on its own, so without this the camera
	// would stop wherever the last pointer event left it and the smoothing
	// would only be visible while the wheel was actually turning.
	dt := 0.0
	if !m.lastFrameAt.IsZero() {
		dt = gtx.Now.Sub(m.lastFrameAt).Seconds()
	}
	m.lastFrameAt = gtx.Now
	if m.stepZoom(dt, sz) {
		gtx.Execute(op.InvalidateCmd{})
		// The camera moved after project ran, so redo it rather than drawing
		// this frame's nodes at last frame's zoom.
		pts = m.project(s, sz)
	}
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
		m.CoverageOpacity.Default = 0.75
		alpha := m.CoverageOpacity.Value()
		ol := paint.PushOpacity(gtx.Ops, alpha)
		m.drawCoverage(t, gtx, sz, s)
		ol.Pop()
		// The ground itself, ghosted back over the raster: every street at
		// every zoom, from the base already on screen - a roads-only source
		// thins out exactly where somebody zooms in to look. Scaled by the
		// raster's own opacity, so backing the raster off does not double
		// up the ground.
		if m.Tiles != nil && m.Layers.Basemap && s.Coverage != nil {
			gl := paint.PushOpacity(gtx.Ops, 0.4*alpha)
			m.Tiles.Draw(gtx, sz, m.CentreLat, m.CentreLon, m.Zoom)
			gl.Pop()
		}
	}
	if m.Layers.Buildings {
		m.drawBuildings(t, gtx, sz)
	}

	// Roads and place names, back on top: a raster that buries the map's
	// structure explains coverage of an anonymous landscape.
	if m.Layers.Basemap && m.Layers.Coverage {
		for _, ov := range m.OverlayTiles {
			ov.Draw(gtx, sz, m.CentreLat, m.CentreLon, m.Zoom)
		}
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
	ink := m.baseInk(t)
	for i, at := range spots {
		col := theme.Alpha(ink, 0.85)
		if pts[i].n.Selected || i == m.cam.hover {
			col = ink
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
	// An empty network says what to do with the map rather than what it has
	// not got: the ground is there to be clicked on.
	if empty {
		layout.Center.Layout(gtx, Text(t, t.Sz.Body, m.baseInk(t),
			"no nodes yet - choose the place tool and click the ground"))
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
	m.linkMark(t, gtx, sz)
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
