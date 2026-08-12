package comp

import (
	"fmt"
	"image"
	"math"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// The furniture around the map: what you can turn off, how far things are,
// and where the picture came from.

// Layers is what the map draws. Every overlay is here rather than in a
// settings file, because the answer to "why can I not see the links" should be
// visible on the same screen as the links.
type Layers struct {
	Basemap    bool
	Boundaries bool
	Links      bool
	WeakLinks  bool
	Nodes      bool
	Labels     bool
	Traffic    bool
	// Measure puts the map in measuring mode, where a drag reports a distance
	// and a bearing instead of panning.
	Measure bool

	set     bool
	toggles [8]Check
}

// defaults are applied once, so a zero Layers is a sensible map rather than an
// empty one.
func (l *Layers) defaults() {
	if l.set {
		return
	}
	l.Basemap, l.Boundaries, l.Links = true, true, true
	l.Nodes, l.Labels, l.Traffic, l.set = true, true, true, true
}

type layerRow struct {
	name string
	on   *bool
}

func (l *Layers) rows() []layerRow {
	return []layerRow{
		{"Basemap", &l.Basemap},
		{"Boundaries", &l.Boundaries},
		{"Links", &l.Links},
		{"Weak links", &l.WeakLinks},
		{"Nodes", &l.Nodes},
		{"Labels", &l.Labels},
		{"Traffic", &l.Traffic},
		{"Measure", &l.Measure},
	}
}

// layerPanel draws the layer switches in the top right of the map.
func (m *MapView) layerPanel(t *theme.Theme, gtx layout.Context, sz image.Point) {
	rows := m.Layers.rows()
	for i := range rows {
		// The checkbox owns the interaction; the layer owns the truth. Read
		// the widget when it changes, and write it back otherwise, so a layer
		// set from anywhere else shows up in the panel.
		c := &m.Layers.toggles[i]
		c.Label = rows[i].name
		if c.Bool.Update(gtx) {
			*rows[i].on = c.Bool.Value
		} else {
			c.Bool.Value = *rows[i].on
		}
	}

	// Measure the rows, then draw a box that fits them.
	//
	// Sized by arithmetic - rows times row height plus inset - the box was a
	// row and a half short, because a checkbox is not a table row and never
	// agreed to be one. Recording the content first costs one macro and
	// cannot be wrong.
	pad := gtx.Dp(t.Sp.S)
	inner := gtx
	inner.Constraints.Min = image.Point{}
	inner.Constraints.Max = image.Pt(gtx.Dp(200), sz.Y)

	rec := op.Record(gtx.Ops)
	var kids []layout.FlexChild
	for i := range rows {
		i := i
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return m.Layers.toggles[i].Layout(t, gtx)
		}))
	}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(inner, kids...)
	content := rec.Stop()

	box := image.Pt(dims.Size.X+pad*2, dims.Size.Y+pad*2)
	at := image.Pt(sz.X-box.X-gtx.Dp(t.Sp.M), gtx.Dp(t.Sp.M))
	off := op.Offset(at).Push(gtx.Ops)
	defer off.Pop()

	paint.FillShape(gtx.Ops, theme.Alpha(t.P.Panel, 0.88), clip.Rect{Max: box}.Op())
	Border(gtx, box, 2, 1, t.P.Rule)

	in := op.Offset(image.Pt(pad, pad)).Push(gtx.Ops)
	content.Add(gtx.Ops)
	in.Pop()
}

// scaleBar draws a bar of a round number of kilometres, and says how round.
//
// The old map drew a bar labelled "20 km" whatever the zoom, which is worse
// than no scale bar: a wrong scale is read as a right one.
func (m *MapView) scaleBar(t *theme.Theme, gtx layout.Context, sz image.Point, note string) {
	// Metres per pixel at the centre latitude, from the pixels-per-degree the
	// camera is actually using.
	mPerPx := 111320 * math.Cos(m.CentreLat*math.Pi/180) / m.Zoom
	if mPerPx <= 0 {
		return
	}
	target := 140.0 // pixels, about a thumb
	metres := niceDistance(target * mPerPx)
	px := int(metres / mPerPx)
	if px < 20 || px > sz.X/2 {
		return
	}

	y := sz.Y - gtx.Dp(t.Sp.XL)
	x := gtx.Dp(t.Sp.M)
	off := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	// A bar with end ticks, so it is read as a measurement rather than a rule.
	paint.FillShape(gtx.Ops, t.P.Dim, clip.Rect{
		Min: image.Pt(0, 0), Max: image.Pt(px, 2)}.Op())
	paint.FillShape(gtx.Ops, t.P.Dim, clip.Rect{
		Min: image.Pt(0, -4), Max: image.Pt(2, 6)}.Op())
	paint.FillShape(gtx.Ops, t.P.Dim, clip.Rect{
		Min: image.Pt(px-2, -4), Max: image.Pt(px, 6)}.Op())
	off.Pop()

	// Above the bar by exactly its own height. Guessing a gap in Dp put the
	// distance on top of the bar it was labelling.
	above(gtx, image.Pt(x, y), gtx.Dp(3),
		Mono(t, t.Sz.Caption, t.P.Dim, distanceLabel(metres)))

	off = op.Offset(image.Pt(x, y+gtx.Dp(t.Sp.S))).Push(gtx.Ops)
	Mono(t, t.Sz.Caption, t.P.Faint, note)(unbounded(gtx))
	off.Pop()
}

// niceDistance rounds to 1, 2 or 5 times a power of ten, which is what a scale
// bar has to be to be read at a glance.
func niceDistance(m float64) float64 {
	if m <= 0 {
		return 1
	}
	pow := math.Pow(10, math.Floor(math.Log10(m)))
	switch r := m / pow; {
	case r >= 5:
		return 5 * pow
	case r >= 2:
		return 2 * pow
	}
	return pow
}

func distanceLabel(metres float64) string {
	if metres >= 1000 {
		return fmt.Sprintf("%g km", metres/1000)
	}
	return fmt.Sprintf("%g m", metres)
}

// measureReadout reports the distance and bearing of the measuring line.
func (m *MapView) measureReadout(t *theme.Theme, gtx layout.Context, sz image.Point) {
	if !m.Layers.Measure || m.cam.drag != dragMeasure || !m.cam.moved {
		return
	}
	a, b := m.cam.from, m.cam.last
	var p clip.Path
	p.Begin(gtx.Ops)
	segment(&p, a, b, 1.5)
	paint.FillShape(gtx.Ops, t.P.Warn, clip.Outline{Path: p.End()}.Op())

	lat1, lon1 := m.unproject(a, sz)
	lat2, lon2 := m.unproject(b, sz)
	d := haversineM(lat1, lon1, lat2, lon2)
	brg := bearing(lat1, lon1, lat2, lon2)

	at := image.Pt(int(b.X)+10, int(b.Y)-8)
	off := op.Offset(at).Push(gtx.Ops)
	Mono(t, t.Sz.Caption, t.P.Ink,
		fmt.Sprintf("%s  %.0f deg", distanceLabel(math.Round(d)), brg))(unbounded(gtx))
	off.Pop()
}

func haversineM(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371000.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	la, lb := lat1*math.Pi/180, lat2*math.Pi/180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(la)*math.Cos(lb)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Sqrt(h))
}

// bearing is the initial great-circle bearing, in degrees from north.
func bearing(lat1, lon1, lat2, lon2 float64) float64 {
	la, lb := lat1*math.Pi/180, lat2*math.Pi/180
	dLon := (lon2 - lon1) * math.Pi / 180
	y := math.Sin(dLon) * math.Cos(lb)
	x := math.Cos(la)*math.Sin(lb) - math.Sin(la)*math.Cos(lb)*math.Cos(dLon)
	deg := math.Atan2(y, x) * 180 / math.Pi
	if deg < 0 {
		deg += 360
	}
	return deg
}

// basemapNote is the attribution line, and the honesty line when the basemap
// is incomplete.
//
// 10.1 asks for an offline mode that fails loudly. Loud here means a sentence
// under the scale bar rather than a dialog: a missing basemap does not stop
// anybody working, but a map that is quietly half-drawn invites somebody to
// read the gaps as geography.
func basemapNote(drawn, want int, on bool) string {
	const attrib = "Elevation: AWS terrarium    (c) OpenStreetMap"
	switch {
	case !on:
		return "basemap off"
	case want == 0:
		return "no basemap: no tile store"
	case drawn == 0:
		return "no basemap: nothing cached for this view, and tiles are never fetched while drawing"
	case drawn < want:
		return fmt.Sprintf("basemap %d of %d tiles cached; the rest are being fetched    %s",
			drawn, want, attrib)
	}
	return attrib
}

// above draws a widget with its bottom edge gap pixels above a point.
func above(gtx layout.Context, at image.Point, gap int, w layout.Widget) {
	m := op.Record(gtx.Ops)
	d := w(unbounded(gtx))
	call := m.Stop()
	off := op.Offset(image.Pt(at.X, at.Y-d.Size.Y-gap)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	off.Pop()
}

// mapNote adds the study margin to the attribution line.
//
// The margin was drawn as a band of circles around the boundary, which
// rendered as a row of visible octagons and read as geography rather than as
// a rule. A boundary is a line and a margin is a number; the honest drawing of
// a number is the number.
func mapNote(s *state.Snapshot, shown, total int, basemap string) string {
	out := ""
	if total > 0 && shown < total {
		out = fmt.Sprintf("showing the %d strongest of %d links    ", shown, total)
	}
	if s != nil && s.MarginKm > 0 {
		out += fmt.Sprintf("study margin %g km    ", s.MarginKm)
	}
	return out + basemap
}
