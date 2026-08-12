package comp

import (
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
			m.zoomAt(e.Position, math.Pow(1.1, float64(-e.Scroll.Y)), sz)
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
			// Any other press dismisses an open menu, which is what clicking
			// away from a menu means everywhere else.
			m.menu.open = false
			switch {
			case m.Layers.Measure:
				m.cam.drag = dragMeasure
			case e.Modifiers.Contain(key.ModShift):
				m.cam.drag, m.cam.boxTo = dragBox, e.Position
			case m.cam.nodeIndex >= 0:
				m.cam.drag = dragNode
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

// zoomAt scales about a screen point, keeping whatever is under it there.
func (m *MapView) zoomAt(at f32.Point, factor float64, sz image.Point) {
	lat, lon := m.unproject(at, sz)
	m.Zoom *= factor
	// A floor and a ceiling, because a zoom of zero divides by zero and a very
	// large one overflows the tile arithmetic.
	m.Zoom = math.Max(2, math.Min(4_000_000, m.Zoom))
	// Put the same place back under the cursor.
	lat2, lon2 := m.unproject(at, sz)
	m.CentreLat += lat - lat2
	m.CentreLon += lon - lon2
	m.clampCentre()
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
