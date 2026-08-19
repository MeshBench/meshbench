package comp

import (
	"time"

	"image"
	"math"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
)

// The camera, and the pointer.
//
// Where somebody is looking is a property of the view rather than of the
// world, so none of this goes through the store. What the pointer *decides* -
// which nodes are selected, where a node was dragged to - does, as a verb,
// because that is a change to the network and everything else has to hear
// about it.

// dragKind is what the pointer is currently doing, decided on the first drag
// event rather than on the press: until it moves, a press on a node is
// indistinguishable from a click.
type dragKind int

const (
	dragNone dragKind = iota
	dragPan
	dragNode
	dragBox
	dragMeasure
)

type camera struct {
	drag      dragKind
	from      f32.Point
	last      f32.Point
	moved     bool
	nodeIndex int
	boxTo     f32.Point
	hover     int
}

// handle processes every pointer event for one frame and returns whether the
// view needs redrawing. It is called before anything is drawn, so a drag is
// shown at its new position in the same frame that produced it.
func (m *MapView) handle(gtx layout.Context, sz image.Point, pts []projected) {
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, m)

	// Escape abandons the link tool's half-made pick, and tells the
	// workbench so - a pinned pair is released the same way.
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			m.CancelLink()
		}
	}

	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: m,
			Kinds: pointer.Press | pointer.Drag | pointer.Release |
				pointer.Move | pointer.Scroll | pointer.Leave,
			ScrollY: pointer.ScrollRange{Min: -20, Max: 20},
		})
		if !ok {
			return
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Scroll:
			// Zoom to the cursor, not to the centre: the thing under the
			// pointer is the thing being looked at, and it should stay put.
			//
			// The wheel moves a *target*, not the camera. A wheel reports in
			// notches, so applying each one on arrival moved the map in the
			// size of a notch however small the multiplier was - the steps
			// were the input's, not the zoom's, and no per-unit constant was
			// ever going to smooth them out. The camera chases this between
			// frames instead.
			m.aimZoom(math.Pow(zoomStep, float64(-e.Scroll.Y)), e.Position, sz)
		case pointer.Move:
			m.cam.hover = nearestWithin(pts, e.Position, 10)
		case pointer.Leave:
			m.cam.hover = -1
		case pointer.Press:
			m.cam.from, m.cam.last, m.cam.moved = e.Position, e.Position, false
			m.cam.nodeIndex = nearestWithin(pts, e.Position, 10)
			// A right-click is a question about what is under the pointer, not
			// a gesture: it opens a menu and starts no drag.
			if e.Buttons.Contain(pointer.ButtonSecondary) {
				m.openMenu(e.Position, pts, sz)
				m.cam.drag = dragNone
				continue
			}
			// A press inside an open menu belongs to the menu.
			//
			// Dismissing on any press dismissed on the press that chose an
			// entry too: the entry registered its click and the menu was shut
			// before the frame that would have acted on it, so choosing
			// anything did nothing at all.
			if m.menu.open && e.Position.X >= float32(m.menu.box.Min.X) &&
				e.Position.X < float32(m.menu.box.Max.X) &&
				e.Position.Y >= float32(m.menu.box.Min.Y) &&
				e.Position.Y < float32(m.menu.box.Max.Y) {
				m.cam.drag = dragNone
				continue
			}
			// Any other press dismisses an open menu, which is what clicking
			// away from a menu means everywhere else.
			m.menu.open = false
			// What a press starts depends on the tool.
			//
			// Nothing read the tool at all: every mode dragged a node, which
			// is why a repeater could be walked across the map while the
			// toolbar said "link", and place and link did nothing whatever.
			switch {
			case m.Tool == "measure" || m.Layers.Measure:
				m.cam.drag = dragMeasure
			case e.Modifiers.Contain(key.ModShift):
				m.cam.drag, m.cam.boxTo = dragBox, e.Position
			case m.Tool == "move" && m.cam.nodeIndex >= 0:
				m.cam.drag = dragNode
			case m.Tool == "place", m.Tool == "link":
				// Both act on the release, so that a press which turns into a
				// drag pans the map instead of placing something nobody
				// meant to place.
				m.cam.drag = dragPan
			default:
				m.cam.drag = dragPan
			}
		case pointer.Drag:
			d := f32.Pt(e.Position.X-m.cam.last.X, e.Position.Y-m.cam.last.Y)
			if d.X != 0 || d.Y != 0 {
				m.cam.moved = true
			}
			m.cam.last = e.Position
			switch m.cam.drag {
			case dragPan:
				m.pan(d, sz)
			case dragBox:
				m.cam.boxTo = e.Position
			case dragMeasure:
				// Nothing to do: the readout is drawn from from and last.
			case dragNode:
				if m.OnMove != nil && m.cam.nodeIndex >= 0 {
					lat, lon := m.unproject(e.Position, sz)
					m.OnMove(pts[m.cam.nodeIndex].n.Name, lat, lon)
				}
			}
		case pointer.Release:
			m.release(e, pts, sz)
			m.cam.drag = dragNone
		}
	}
}

// release is where a gesture becomes a decision.
func (m *MapView) release(e pointer.Event, pts []projected, sz image.Point) {
	// The tools that act on a click rather than on a drag.
	//
	// Checked before anything else and only when the pointer did not move,
	// because a press that turned into a pan is somebody looking around, not
	// somebody placing a repeater in the sea.
	if !m.cam.moved {
		switch m.Tool {
		case "place":
			if m.OnPlace != nil {
				lat, lon := m.unproject(e.Position, sz)
				m.OnPlace(lat, lon)
			}
			return
		case "link":
			m.linkClick(e.Position, pts, sz)
			return
		}
	}
	switch {
	case m.cam.drag == dragMeasure:
		// A measurement is not a selection.
	case m.cam.drag == dragBox:
		// A box that was never dragged is a shift-click, which is an additive
		// click rather than an empty selection.
		if !m.cam.moved {
			m.selectAt(e.Position, pts, true)
			return
		}
		r := boxOf(m.cam.from, m.cam.boxTo)
		var names []string
		for _, p := range pts {
			if image.Pt(int(p.x), int(p.y)).In(r) {
				names = append(names, p.n.Name)
			}
		}
		if m.OnSelect != nil {
			m.OnSelect(names, true)
		}
	case m.cam.moved:
		// A drag is not a click. Releasing after moving the map or a node must
		// not also change what is selected.
	default:
		// Two clicks on the same node inside half a second open its window.
		// Half a second is the double-click convention everywhere, and the
		// name has to match: two fast clicks on two different nodes are two
		// selections, not a request to open either.
		if i := nearestWithin(pts, e.Position, 10); i >= 0 {
			name := pts[i].n.Name
			if name == m.lastClickName && e.Time-m.lastClickAt < 500*time.Millisecond {
				m.lastClickName = ""
				if m.OnNodeOpen != nil {
					m.OnNodeOpen(name)
				}
				return
			}
			m.lastClickName, m.lastClickAt = name, e.Time
		} else {
			m.lastClickName = ""
		}
		m.selectAt(e.Position, pts, false)
	}
}

// selectAt selects whatever is under a point, or nothing.
func (m *MapView) selectAt(at f32.Point, pts []projected, additive bool) {
	if m.OnSelect == nil {
		return
	}
	if i := nearestWithin(pts, at, 10); i >= 0 {
		m.OnSelect([]string{pts[i].n.Name}, additive)
		return
	}
	if !additive {
		m.OnSelect(nil, false)
	}
}

// pan moves the camera by a pixel offset.
func (m *MapView) pan(d f32.Point, sz image.Point) {
	cos := math.Cos(m.CentreLat * math.Pi / 180)
	if cos < 0.01 {
		cos = 0.01
	}
	m.CentreLon -= float64(d.X) / (m.Zoom * cos)
	m.CentreLat += float64(d.Y) / m.Zoom
	m.clampCentre()
}

// zoomStep is the multiplier one unit of scroll applies.
//
// 1.1 read as a jump rather than a zoom: a fast wheel spin or a trackpad
// gesture can hand a single event a Scroll.Y in the twenties, and 1.1^20 is
// nearly 7x in one event. This is the same feel a per-unit multiplier this
// close to 1 gives everywhere else - the map just moves in smaller, more
// controllable steps.
const zoomStep = 1.03

// aimZoom points the zoom somewhere without going there yet.
//
// Multiplying the target rather than the camera is what makes a fast spin add
// up: three notches in quick succession compose into one longer glide instead
// of three separate jumps.
func (m *MapView) aimZoom(factor float64, at f32.Point, sz image.Point) {
	if m.zoomTarget == 0 || !m.zooming {
		m.zoomTarget = m.Zoom
	}
	m.zoomTarget = clampZoom(m.zoomTarget * factor)
	// The ground under the pointer, taken now and held for the whole glide.
	// Every frame then places *this* position back under the cursor, which is
	// exact. Nudging the centre by the difference each frame instead drifts:
	// the longitude scale depends on the latitude being corrected, so the
	// correction changes its own units as it goes.
	m.zoomAnchor = at
	m.anchorLat, m.anchorLon = m.unproject(at, sz)
	m.zooming = true
}

// stepZoom moves the camera part of the way to the target, and reports
// whether it is still going.
//
// Exponential easing - a fixed fraction of what is left, per second - so it
// starts quickly and settles rather than arriving at a hard stop. Scaled by
// the frame's own elapsed time, so it takes the same wall-clock time to get
// there whether the window is managing 120 frames a second or 30.
func (m *MapView) stepZoom(dt float64, sz image.Point) bool {
	if !m.zooming {
		return false
	}
	if m.zoomTarget == 0 {
		m.zoomTarget = m.Zoom
	}
	// Close enough that another frame would not be visible: a fraction of a
	// percent of the current zoom, rather than an absolute figure that would
	// be far too fine at 4,000,000 and far too coarse at 2.
	if math.Abs(m.zoomTarget-m.Zoom) <= m.Zoom*0.0005 {
		m.holdAnchor(m.zoomTarget, sz)
		m.zooming = false
		return false
	}
	if dt <= 0 || dt > 0.25 {
		// A first frame has no previous timestamp, and a window that was
		// asleep hands back an enormous one. Neither should teleport the
		// camera, so they take a nominal step instead.
		dt = 1.0 / 60
	}
	k := 1 - math.Exp(-zoomChaseRate*dt)
	m.holdAnchor(m.Zoom+(m.zoomTarget-m.Zoom)*k, sz)
	return true
}

// holdAnchor sets the zoom and moves the centre so the anchored position sits
// exactly under the anchored screen point.
//
// Solved rather than nudged. unproject reads
//
//	lat = CentreLat - (y - h/2)/Zoom
//	lon = CentreLon + (x - w/2)/(Zoom*cos(CentreLat))
//
// so the centre that puts a known lat/lon at a known point is those two lines
// rearranged - latitude first, because longitude's scale depends on it.
func (m *MapView) holdAnchor(z float64, sz image.Point) {
	m.Zoom = clampZoom(z)
	m.CentreLat = m.anchorLat + (float64(m.zoomAnchor.Y)-float64(sz.Y)/2)/m.Zoom
	m.CentreLat = math.Max(-85, math.Min(85, m.CentreLat))
	cos := math.Cos(m.CentreLat * math.Pi / 180)
	if cos < 0.01 {
		cos = 0.01
	}
	m.CentreLon = m.anchorLon - (float64(m.zoomAnchor.X)-float64(sz.X)/2)/(m.Zoom*cos)
	m.clampCentre()
}

// zoomChaseRate is how fast the camera closes on the target, per second, as
// the exponent of the easing. Around 18 lands in roughly a fifth of a second:
// slow enough to read as movement rather than a cut, fast enough that the map
// is never lagging behind the hand on the wheel.
const zoomChaseRate = 18.0

// clampZoom is the floor and ceiling, because a zoom of zero divides by zero
// and a very large one overflows the tile arithmetic.
func clampZoom(z float64) float64 {
	return math.Max(2, math.Min(4_000_000, z))
}

// clampCentre keeps the camera on the planet.
func (m *MapView) clampCentre() {
	m.CentreLat = math.Max(-85, math.Min(85, m.CentreLat))
	for m.CentreLon > 180 {
		m.CentreLon -= 360
	}
	for m.CentreLon < -180 {
		m.CentreLon += 360
	}
}

// unproject is the inverse of project: a screen point back to a position.
func (m *MapView) unproject(at f32.Point, sz image.Point) (lat, lon float64) {
	cos := math.Cos(m.CentreLat * math.Pi / 180)
	if cos < 0.01 {
		cos = 0.01
	}
	lon = m.CentreLon + (float64(at.X)-float64(sz.X)/2)/(m.Zoom*cos)
	lat = m.CentreLat - (float64(at.Y)-float64(sz.Y)/2)/m.Zoom
	return lat, lon
}

// CentreOn puts a position in the middle of the view without changing zoom.
// ViewportBox is the ground the map currently shows, for the callers that
// cannot reach into a frame. Before the first frame there is no viewport,
// and it says so.
func (m *MapView) ViewportBox() (south, west, north, east float64, ok bool) {
	if m.lastSize.X == 0 || m.lastSize.Y == 0 {
		return 0, 0, 0, 0, false
	}
	south, west = m.unproject(f32.Pt(0, float32(m.lastSize.Y)), m.lastSize)
	north, east = m.unproject(f32.Pt(float32(m.lastSize.X), 0), m.lastSize)
	return south, west, north, east, true
}

// viewportCells is the raster resolution that looks right on THIS screen:
// about two and a half pixels per cell - sharp without pretending to more
// than the display can show - which also means a zoomed-out viewport costs
// the same as a zoomed-in one, with the metres per cell doing the scaling.
func (m *MapView) viewportCells(screenPx int) int {
	cells := int(float64(screenPx) / 2.5)
	if cells < 64 {
		cells = 64
	}
	if cells > 2048 {
		cells = 2048
	}
	return cells
}

// ViewportCells is viewportCells for the window the map last drew into,
// for callers outside the frame (the menu bar).
func (m *MapView) ViewportCells() int {
	return m.viewportCells(m.lastSize.X)
}

// StartAt pins the camera before the first frame - the capture flags' way
// in, since the first frame otherwise fits the whole network over whatever
// the flags asked for.
func (m *MapView) StartAt(lat, lon, zoom float64) {
	m.CentreLat, m.CentreLon = lat, lon
	m.Zoom, m.zoomTarget = zoom, zoom
	m.initialised, m.FitNext = true, false
}

func (m *MapView) CentreOn(lat, lon float64) {
	m.CentreLat, m.CentreLon = lat, lon
	m.clampCentre()
}

// nearestWithin is the node closest to a point, if one is close enough to have
// been meant. Squared distances, because the comparison does not need the
// square root and this runs on every mouse move.
func nearestWithin(pts []projected, at f32.Point, radius float32) int {
	best, bestD := -1, radius*radius
	for i, p := range pts {
		dx, dy := p.x-at.X, p.y-at.Y
		if d := dx*dx + dy*dy; d <= bestD {
			best, bestD = i, d
		}
	}
	return best
}

func boxOf(a, b f32.Point) image.Rectangle {
	r := image.Rect(int(a.X), int(a.Y), int(b.X), int(b.Y))
	return r.Canon()
}

// openMenu puts the context menu under the pointer.
func (m *MapView) openMenu(at f32.Point, pts []projected, sz image.Point) {
	name := ""
	if i := nearestWithin(pts, at, 10); i >= 0 {
		name = pts[i].n.Name
	}
	lat, lon := m.unproject(at, sz)
	m.menu = mapMenu{
		open: true, at: image.Pt(int(at.X), int(at.Y)),
		node: name, lat: lat, lon: lon, items: menuFor(name),
	}
}
