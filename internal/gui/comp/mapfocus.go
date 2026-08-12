package comp

import (
	"image"
	"math"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// What a selected node tells you: its antenna, its neighbours, its regions.

// drawPattern draws the antenna pattern of each selected node, rotated to its
// bearing (10.8a).
//
// A polar plot in real bearings, scaled so the peak is a fixed number of
// pixels. Not scaled to distance: gain is not range, and drawing it as though
// it were would put a number on the map that nothing in the model supports.
func (m *MapView) drawPattern(t *theme.Theme, gtx layout.Context, pts []projected,
	sz image.Point) {

	const peakPx = 34
	var path clip.Path
	path.Begin(gtx.Ops)
	n := 0
	for _, p := range pts {
		if !p.n.Selected || len(p.n.Pattern) < 3 || offscreen(p, sz) {
			continue
		}
		// Normalise against the peak, and floor at 20 dB down: below that the
		// plot is all noise and the shape stops meaning anything.
		peak := p.n.Pattern[0]
		for _, g := range p.n.Pattern {
			peak = math.Max(peak, g)
		}
		var prev f32.Point
		for i := 0; i <= len(p.n.Pattern); i++ {
			g := p.n.Pattern[i%len(p.n.Pattern)]
			rel := math.Max(0, 1+(g-peak)/20)
			// Compass bearing: zero is north, and north is up.
			ang := float64(i%len(p.n.Pattern)) * 2 * math.Pi / float64(len(p.n.Pattern))
			cur := f32.Pt(
				p.x+float32(math.Sin(ang)*rel*peakPx),
				p.y-float32(math.Cos(ang)*rel*peakPx),
			)
			if i > 0 {
				segment(&path, prev, cur, 1.2)
				n++
			}
			prev = cur
		}
	}
	spec := path.End()
	if n > 0 {
		paint.FillShape(gtx.Ops, theme.Alpha(t.P.Selected, 0.75),
			clip.Outline{Path: spec}.Op())
	}
}

// drawNeighbours picks out the links of the selected nodes (10.8c).
//
// Drawn over the ordinary links rather than instead of them, so the answer to
// "who can this node hear" is read against the rest of the mesh rather than in
// place of it.
func (m *MapView) drawNeighbours(t *theme.Theme, gtx layout.Context, pts []projected,
	sz image.Point, s *state.Snapshot) {

	sel := map[int]bool{}
	for i, p := range pts {
		if p.n.Selected {
			sel[i] = true
		}
	}
	if len(sel) == 0 {
		return
	}
	var path clip.Path
	path.Begin(gtx.Ops)
	n := 0
	for _, l := range s.Links {
		if !l.Known || (!sel[l.A] && !sel[l.B]) {
			continue
		}
		if l.A >= len(pts) || l.B >= len(pts) || l.MarginDB < 0 {
			continue
		}
		a, b := pts[l.A], pts[l.B]
		if offscreen(a, sz) && offscreen(b, sz) {
			continue
		}
		segment(&path, f32.Pt(a.x, a.y), f32.Pt(b.x, b.y), 2)
		n++
	}
	spec := path.End()
	if n > 0 {
		paint.FillShape(gtx.Ops, theme.Alpha(t.P.Selected, 0.85),
			clip.Outline{Path: spec}.Op())
	}
}

// drawRegions rings each node in a colour derived from its first region
// (10.8b).
//
// Derived rather than assigned, because regions are discovered from a live
// network and there is no fixed list to assign colours from. The same region
// is the same colour in every run, which is what matters.
func (m *MapView) drawRegions(t *theme.Theme, gtx layout.Context, pts []projected,
	sz image.Point) {

	byRegion := map[string][]projected{}
	for _, p := range pts {
		if offscreen(p, sz) || len(p.n.Regions) == 0 {
			continue
		}
		byRegion[p.n.Regions[0]] = append(byRegion[p.n.Regions[0]], p)
	}
	for region, list := range byRegion {
		var path clip.Path
		path.Begin(gtx.Ops)
		for _, p := range list {
			ring(&path, f32.Pt(p.x, p.y), 6.5, 1.5)
		}
		spec := path.End()
		paint.FillShape(gtx.Ops, theme.Alpha(regionColour(region), 0.85),
			clip.Outline{Path: spec}.Op())
	}
}

// regionColour hashes a region name to a hue.
//
// Deliberately not the node-kind scale: a node's kind and its region are two
// different facts, and reusing the palette would make them look like one.
func regionColour(name string) (c colorNRGBA) {
	h := uint32(2166136261)
	for i := 0; i < len(name); i++ {
		h ^= uint32(name[i])
		h *= 16777619
	}
	// Spread over the hue circle, at a fixed saturation and value so that no
	// region is drawn in a colour too dark to see against the map.
	return hsv(float64(h%360), 0.55, 0.95)
}
