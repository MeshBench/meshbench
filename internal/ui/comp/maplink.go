// The link tool's two-click state: picking a pair of ends, arming the first,
// abandoning a half-made pick, and drawing the armed end. Split from
// mapinput.go and mapview.go at the file limit.
package comp

import (
	"image"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// LinkEnd is one end of a link being asked about: a node by name, or a bare
// place on the ground. A node end carries its position too, so a consumer
// can draw it without a lookup.
type LinkEnd struct {
	Node     string
	Lat, Lon float64
}

// linkClick is one click of the link tool.
//
// A node when one is under the click, else the ground itself: a link can be
// asked about between two repeaters, two bare places, or one of each. Open
// ground used to abandon the pick, which made the most interesting question
// - "would a mast here reach that" - unaskable.
func (m *MapView) linkClick(at f32.Point, pts []projected, sz image.Point) {
	var end LinkEnd
	if i := nearestWithin(pts, at, 10); i >= 0 {
		n := pts[i].n
		end = LinkEnd{Node: n.Name, Lat: n.Lat, Lon: n.Lon}
	} else {
		lat, lon := m.unproject(at, sz)
		end = LinkEnd{Lat: lat, Lon: lon}
	}
	if m.linkFrom == nil || *m.linkFrom == end {
		m.linkFrom = &end
		if end.Node != "" && m.OnSelect != nil {
			m.OnSelect([]string{end.Node}, false)
		}
		if m.OnLinkArmed != nil {
			m.OnLinkArmed(end)
		}
		return
	}
	if m.OnLinkPair != nil {
		m.OnLinkPair(*m.linkFrom, end)
	}
	m.linkFrom = nil
}

// ArmLink sets the first end of an asked-about link from outside the click
// path - the context menu's "link from here", on a node or on the ground.
func (m *MapView) ArmLink(node string, lat, lon float64) {
	end := LinkEnd{Node: node, Lat: lat, Lon: lon}
	m.linkFrom = &end
	if m.OnLinkArmed != nil {
		m.OnLinkArmed(end)
	}
}

// LinkTo completes a link at this place against the armed first end. With no
// first end it arms this one instead, which is the half of the gesture that
// was actually performed.
func (m *MapView) LinkTo(node string, lat, lon float64) {
	end := LinkEnd{Node: node, Lat: lat, Lon: lon}
	if m.linkFrom == nil || *m.linkFrom == end {
		m.ArmLink(node, lat, lon)
		return
	}
	if m.OnLinkPair != nil {
		m.OnLinkPair(*m.linkFrom, end)
	}
	m.linkFrom = nil
}

// CancelLink abandons the link tool's half-made pick and announces it, so a
// pinned pair is released too. Called on Escape and on a tool change - a
// pick that survives switching to the pan tool is the measure-tool bug all
// over again.
func (m *MapView) CancelLink() {
	if m.linkFrom == nil && !m.PinnedLink {
		return
	}
	m.linkFrom = nil
	m.PinnedLink = false
	if m.OnLinkCancel != nil {
		m.OnLinkCancel()
	}
}

// linkMark draws the link tool's armed first end, on a node or on bare
// ground: a half-made pick that draws nothing reads as a click that did
// nothing.
func (m *MapView) linkMark(t *theme.Theme, gtx layout.Context, sz image.Point) {
	if m.linkFrom == nil {
		return
	}
	at := m.projectPoint(state.Point{Lat: m.linkFrom.Lat, Lon: m.linkFrom.Lon}, sz)
	var path clip.Path
	path.Begin(gtx.Ops)
	ring(&path, at, 11, 2)
	paint.FillShape(gtx.Ops, t.P.Accent, clip.Outline{Path: path.End()}.Op())
}
