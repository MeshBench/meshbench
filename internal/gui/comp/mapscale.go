// The map's scale bar and its distance formatting. Split from
// mapchrome.go at the file limit.
package comp

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/MeshBench/meshbench/internal/geo"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

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
	brg := geo.BearingDeg(lat1, lon1, lat2, lon2)

	at := image.Pt(int(b.X)+10, int(b.Y)-8)
	off := op.Offset(at).Push(gtx.Ops)
	Mono(t, t.Sz.Caption, t.P.Ink,
		fmt.Sprintf("%s  %.0f deg", distanceLabel(math.Round(d)), brg))(unbounded(gtx))
	off.Pop()
}

// haversineM is geo.DistanceKm in the metres a scale bar is drawn in.
func haversineM(lat1, lon1, lat2, lon2 float64) float64 {
	return geo.DistanceKm(lat1, lon1, lat2, lon2) * 1000
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
