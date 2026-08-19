package comp

import (
	"image"

	"gioui.org/f32"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"
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
					m.CoverageOpacity.Default = 0.75
					return m.CoverageOpacity.Layout(t, gtx)
				}),
			)
		}))
		if m.OnRasterView != nil {
			if m.rasterViewBtn.Clicked(gtx) {
				south, west := m.unproject(f32.Pt(0, float32(sz.Y)), sz)
				north, east := m.unproject(f32.Pt(float32(sz.X), 0), sz)
				m.OnRasterView(south, west, north, east, m.viewportCells(sz.X))
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
