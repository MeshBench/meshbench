package comp

import (
	"fmt"
	"gioui.org/widget/material"
	"image"
	"image/color"
	"math"
	"sort"

	"gioui.org/f32"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
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
	Nodes      bool
	Labels     bool
	Traffic    bool
	Coverage   bool
	Terrain    bool
	Buildings  bool
	Regions    bool
	Pattern    bool
	// Measure puts the map in measuring mode, where a drag reports a distance
	// and a bearing instead of panning.
	Measure bool

	// HideKind switches off one kind of node, from the key. The key is where
	// somebody looks to find out what a colour means, so it is also where
	// they reach to stop drawing it.
	HideKind [6]bool

	// HideMaterial switches one building material off, from the key.
	HideMaterial [8]bool

	set     bool
	toggles [12]Check
	keys    [6]widget.Clickable
	matKeys [8]widget.Clickable
	// baseRow is the basemap picker at the top of the panel.
	baseRow widget.Clickable
}

// defaults are applied once, so a zero Layers is a sensible map rather than an
// empty one.
func (l *Layers) defaults() {
	if l.set {
		return
	}
	// Links start off. On the shipped fixtures that layer is 1,619 lines on
	// Fife and 1,223 on Scotland and Ireland, which is most of what is on
	// screen before anybody has asked a question - the network reads as a
	// mesh of green rather than as a set of nodes in places. Turn it on when
	// the question is about reach.
	l.Basemap, l.Boundaries = true, true
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
		{"Nodes", &l.Nodes},
		{"Labels", &l.Labels},
		{"Traffic", &l.Traffic},
		{"Coverage", &l.Coverage},
		{"Terrain", &l.Terrain},
		{"Buildings", &l.Buildings},
		{"Regions", &l.Regions},
		{"Antenna", &l.Pattern},
		{"Measure", &l.Measure},
	}
}

// kindNames name the six node kinds, in the order theme.NodeKind declares
// them, so a colour on the map can be read off the key.
var kindNames = [6]string{
	"Repeater", "Advanced repeater", "Companion", "Room server",
	"Observer", "Emitter",
}

// keyRows is the key: every kind the loaded network actually contains, with
// its colour and whether it is being drawn.
//
// Only the kinds present, because a key naming six things when the map has
// three is a legend for a different map.
func (m *MapView) keyRows(t *theme.Theme, gtx layout.Context,
	s *state.Snapshot) []layout.FlexChild {
	if s == nil {
		return nil
	}
	var present [6]bool
	for i := range s.Nodes {
		k := kindOf(s.Nodes[i].Kind)
		if int(k) >= 0 && int(k) < len(present) {
			present[k] = true
		}
	}
	var kids []layout.FlexChild
	first := true
	for k := range present {
		if !present[k] {
			continue
		}
		k := k
		if first {
			first = false
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
					Text(t, t.Sz.Caption, t.P.Faint, "key - click to hide"))
			}))
		}
		if m.Layers.keys[k].Clicked(gtx) {
			m.Layers.HideKind[k] = !m.Layers.HideKind[k]
		}
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return m.Layers.keys[k].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				col, ink := t.NodeColour(theme.NodeKind(k)), t.P.Ink
				if m.Layers.HideKind[k] {
					col, ink = theme.Alpha(col, 0.25), t.P.Faint
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						d := gtx.Dp(10)
						paint.FillShape(gtx.Ops, col, clip.Ellipse{
							Max: image.Pt(d, d),
						}.Op(gtx.Ops))
						return layout.Dimensions{Size: image.Pt(d+gtx.Dp(t.Sp.XS), d)}
					}),
					layout.Rigid(Text(t, t.Sz.Caption, ink, kindNames[k])),
				)
			})
		}))
	}
	return kids
}

// layerPanel draws the layer switches in the top right of the map.
func (m *MapView) layerPanel(t *theme.Theme, gtx layout.Context, sz image.Point,
	s *state.Snapshot) {
	rows := m.Layers.rows()
	for i := range rows {
		// The checkbox owns the interaction; the layer owns the truth. Read
		// the widget when it changes, and write it back otherwise, so a layer
		// set from anywhere else shows up in the panel.
		c := &m.Layers.toggles[i]
		c.Label = rows[i].name
		if c.Bool.Update(gtx) {
			was := *rows[i].on
			*rows[i].on = c.Bool.Value
			// Turning a layer on can be a request for something that has to
			// be computed. The map does not compute it: it says so, and the
			// store decides.
			if !was && c.Bool.Value && m.OnLayerOn != nil {
				m.OnLayerOn(rows[i].name)
			}
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

	if m.Layers.baseRow.Clicked(gtx) && m.OnBasemap != nil {
		m.OnBasemap()
	}

	rec := op.Record(gtx.Ops)
	var kids []layout.FlexChild
	// The basemap picker first: which map is under everything is the first
	// question about the map, and the old workbench answered it here too.
	if m.Tiles != nil {
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return m.Layers.baseRow.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				name := m.Tiles.Layer.Name
				ink := t.P.Ink
				if m.Layers.baseRow.Hovered() {
					ink = t.P.Accent
				}
				return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(Text(t, t.Sz.Caption, t.P.Faint, "map:  ")),
							layout.Rigid(Text(t, t.Sz.Caption, ink, name)),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Left: t.Sp.XS}.Layout(gtx,
									func(gtx layout.Context) layout.Dimensions {
										return chevronDown(t, gtx)
									})
							}),
						)
					})
			})
		}))
	}
	for i := range rows {
		i := i
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return m.Layers.toggles[i].Layout(t, gtx)
		}))
	}
	// Coverage's own controls, right under its switch: the opacity slider
	// and the raster-this-view button only exist while the layer does.
	if m.Layers.Coverage {
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(130)
			gtx.Constraints.Max.X = gtx.Dp(130)
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(Text(t, t.Sz.Caption, t.P.Faint, "opacity ")),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if m.CoverageOpacity.Value == 0 {
						m.CoverageOpacity.Value = 1
					}
					return material.Slider(t.M, &m.CoverageOpacity).Layout(gtx)
				}),
			)
		}))
		if m.OnRasterView != nil {
			if m.rasterViewBtn.Clicked(gtx) {
				south, west := m.unproject(f32.Pt(0, float32(sz.Y)), sz)
				north, east := m.unproject(f32.Pt(float32(sz.X), 0), sz)
				m.OnRasterView(south, west, north, east)
			}
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return m.rasterViewBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					ink := t.P.Accent
					if m.rasterViewBtn.Hovered() {
						ink = t.P.Ink
					}
					return Text(t, t.Sz.Caption, ink, "raster this view")(gtx)
				})
			}))
		}
	}
	kids = append(kids, m.keyRows(t, inner, s)...)
	for _, row := range m.buildingKeyRows(t) {
		row := row
		kids = append(kids, layout.Rigid(row))
	}
	kids = append(kids, m.regionKeyRows(t, inner, s)...)
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
	// The ink the basemap needs, not the ink the theme has: near-white on a
	// dark layer, near-black on a light one.
	ink := m.baseInk(t)

	off := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	// A bar with end ticks, so it is read as a measurement rather than a rule.
	paint.FillShape(gtx.Ops, ink, clip.Rect{
		Min: image.Pt(0, 0), Max: image.Pt(px, 2)}.Op())
	paint.FillShape(gtx.Ops, ink, clip.Rect{
		Min: image.Pt(0, -4), Max: image.Pt(2, 6)}.Op())
	paint.FillShape(gtx.Ops, ink, clip.Rect{
		Min: image.Pt(px-2, -4), Max: image.Pt(px, 6)}.Op())
	off.Pop()

	// Above the bar by exactly its own height. Guessing a gap in Dp put the
	// distance on top of the bar it was labelling.
	above(gtx, image.Pt(x, y), gtx.Dp(3),
		Mono(t, t.Sz.Caption, ink, distanceLabel(metres)))

	off = op.Offset(image.Pt(x, y+gtx.Dp(t.Sp.S))).Push(gtx.Ops)
	Mono(t, t.Sz.Caption, theme.Alpha(ink, 0.85), note)(unbounded(gtx))
	off.Pop()
}

// baseInk is the ink that reads on the current basemap: the layer says
// whether its ground is dark; with the basemap off, the theme's own ground
// is what shows, so the theme decides.
func (m *MapView) baseInk(t *theme.Theme) color.NRGBA {
	if m.Tiles != nil && m.Layers.Basemap {
		if m.Tiles.Layer.Dark {
			return t.P.MapInk
		}
		return t.P.MapInkDark
	}
	if t.Mode == theme.Light {
		return t.P.MapInkDark
	}
	return t.P.MapInk
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
func basemapNote(drawn, want int, on bool, layerAttrib string) string {
	// The layer's own credit, not a constant: the satellite imagery is not
	// OpenStreetMap's and must not be credited as if it were.
	attrib := "Elevation: AWS terrarium"
	if layerAttrib != "" {
		attrib += "    " + layerAttrib
	}
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

// regionKeyRows names the regions being drawn, while they are being drawn.
//
// The ring around a node is a hash of its region's name, so without this the
// layer is a set of coloured rings that mean something to nobody. Only while
// the layer is on, and only the regions actually on screen.
func (m *MapView) regionKeyRows(t *theme.Theme, gtx layout.Context,
	s *state.Snapshot) []layout.FlexChild {
	if !m.Layers.Regions || s == nil {
		return nil
	}
	seen := map[string]bool{}
	var names []string
	for i := range s.Nodes {
		// Every region a node holds, not only its first: the rings are
		// concentric now, and a key that names a quarter of them is a key
		// for a different map.
		for _, r := range s.Nodes[i].Regions {
			if !seen[r] {
				seen[r] = true
				names = append(names, r)
			}
		}
	}
	if len(names) == 0 {
		return []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.XS}.Layout(gtx,
				Text(t, t.Sz.Caption, t.P.Faint, "no node here holds a region"))
		})}
	}
	sort.Strings(names)
	kids := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
			Text(t, t.Sz.Caption, t.P.Faint, "regions"))
	})}
	for _, name := range names {
		name := name
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					d := gtx.Dp(10)
					var path clip.Path
					path.Begin(gtx.Ops)
					ring(&path, f32.Pt(float32(d)/2, float32(d)/2), float32(d)/2, 1.5)
					paint.FillShape(gtx.Ops, theme.Alpha(regionColour(name), 0.85),
						clip.Outline{Path: path.End()}.Op())
					return layout.Dimensions{Size: image.Pt(d+gtx.Dp(t.Sp.XS), d)}
				}),
				layout.Rigid(Text(t, t.Sz.Caption, t.P.Ink, name)),
			)
		}))
	}
	return kids
}
