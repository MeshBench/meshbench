// Painting a hopGraph, whichever of the three layouts placed it.
package workbench

import (
	"image"
	"math"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// dimAlpha is how far a reason's usual colour fades once a click has
// focused a different node - visible enough to still read as "this exists",
// faint enough that the focused neighbourhood is unmistakably the point.
const dimAlpha = 0.16

// focusNeighbours is the origin plus whoever a focused node touches by any
// edge, in either direction - what stays at full strength once a click asks
// to see one path rather than the whole mesh.
func focusNeighbours(g hopGraph, focus string) map[string]bool {
	set := map[string]bool{}
	if focus == "" {
		return set
	}
	set[focus] = true
	for _, e := range g.Edges {
		if e.From == focus {
			set[e.To] = true
		}
		if e.To == focus {
			set[e.From] = true
		}
	}
	return set
}

// labelWidth is how much room a node's name gets, and therefore how much
// space the drawing reserves on its right.
const labelWidth = 104

// graphMinHeight is the least the picture is ever allowed to shrink to. A
// free-form or radial shape needs real room to spread into - cramped into a
// strip built for a hop column, either one settles into a tiny cluster in
// the middle of empty space - so this is a floor under wantH, not a target:
// the Packet window can still be resized down to something absurd.
const graphMinHeight = 240

// drawHopGraph paints the graph into the space it is given.
//
// A fixed fraction of the Packet window's own height, not the flexed
// remainder of it: this sits above the journey table, and a graph that grows
// until the table is off the bottom of the window is worse than one you can
// drag to see more of. wantH is that fraction in pixels, chosen by the
// caller from the window's own size rather than from whatever the graph's
// siblings left over, so resizing the window resizes the picture with it.
func drawHopGraph(t *theme.Theme, gtx layout.Context, g hopGraph, v *graphView, wantH int) layout.Dimensions {
	h := wantH
	if min := gtx.Dp(graphMinHeight); h < min {
		h = min
	}
	w := gtx.Constraints.Max.X
	sz := image.Pt(w, h)
	if len(g.Nodes) == 0 || w <= 0 {
		return layout.Dimensions{Size: image.Pt(w, 0)}
	}

	// Free-form keeps its own physics running - a small wiggle even once
	// settled, and a dragged node pushing its neighbours live - which needs a
	// frame to keep arriving on its own rather than only when something else
	// asks for one. Gio does not redraw spontaneously, so this is what makes
	// that true. Roughly 30fps: fast enough to read as motion, cheap enough
	// that a Packet window nobody is looking at does not busy-loop forever
	// once the wiggle would otherwise settle to a stop.
	if v.mode == modeForce {
		gtx.Execute(op.InvalidateCmd{At: time.Now().Add(33 * time.Millisecond)})
	}

	// Positions before input: a click needs to know where things are to
	// hit-test against, and a fresh fit needs them to know what to frame.
	logical := positionsFor(gtx, v, g, w, h)
	if !v.fitted {
		v.pan, v.zoom = fitPositions(logical, w, h)
		v.fitted = true
	}
	if v.focus != "" {
		if _, ok := logical[v.focus]; !ok {
			// The node the last click landed on is not in this build of the
			// graph any more - a hop-depth filter or a reason toggle can do
			// that - so there is nothing left to focus.
			v.focus = ""
		}
	}
	v.handle(gtx, sz, g, logical)
	if v.zoom == 0 {
		v.zoom = 1
	}
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()

	pos := func(name string) (f32.Point, bool) {
		p, ok := logical[name]
		if !ok {
			return f32.Point{}, false
		}
		return f32.Pt(p.X*v.zoom+v.pan.X, p.Y*v.zoom+v.pan.Y), true
	}
	focusSet := focusNeighbours(g, v.focus)
	// A click's focus wins over a hover: they answer different questions
	// ("show me this node's whole neighbourhood" vs. "which line is that"),
	// and running both at once would dim by two different rules on the same
	// frame. Hover only gets a say once nothing is clicked.
	dimmed := func(a, b string) bool {
		if v.focus != "" {
			return !focusSet[a] || !focusSet[b]
		}
		if v.pinFrom != "" {
			return a != v.pinFrom || b != v.pinTo
		}
		if v.hoverFrom != "" {
			return a != v.hoverFrom || b != v.hoverTo
		}
		return false
	}

	// The rings first of everything, so edges and nodes both sit on top of
	// them: a radial layout that never shows its own rings reads as an
	// unexplained tangle rather than as "distance from the origin."
	if v.mode == modeRadial {
		drawRadialRings(t, gtx, g, v, w, h)
	}

	// Edges first, so nodes sit on top of them, and one path per reason -
	// dimmed or not - so each gets its own colour.
	//
	// One clip.Path at a time: Gio allows only a single one open on an Ops, and
	// beginning a second before the first ends panics with "cannot mix multi
	// ops with single ones" — a crash on screen and nothing at all in a unit
	// test. So each pass fully Begins and Ends before the next one starts,
	// never two open together.
	//
	// Reasons reversed so the successes are painted last and sit on top of
	// the misses; within a reason, dimmed first so focused edges land on top
	// of everything that is not part of the answer.
	// Curved rather than straight, in radial mode only: a dead-straight chord
	// through a ring of nodes crosses every ring between the two endpoints and
	// reads as noise cutting through the picture, where a line bowed toward
	// the origin stays legible as "this is a path between two points on rings"
	// even when a hundred of them overlap. Columns and free-form have no
	// centre for a bow to mean anything, so they keep the cheap straight quad.
	center := f32.Pt(float32(w)/2*v.zoom+v.pan.X, float32(h)/2*v.zoom+v.pan.Y)
	curved := v.mode == modeRadial
	at := func(pa, pb f32.Point, f float32) f32.Point {
		return curveEdge(curved, center, pa, pb, f)
	}
	for i := len(missKinds) - 1; i >= 0; i-- {
		mk := missKinds[i]
		col, width, dashed := colourOf(t, mk.Kind)
		for _, dim := range [2]bool{true, false} {
			var p clip.Path
			p.Begin(gtx.Ops)
			drawn := 0
			for _, e := range g.Edges {
				if e.Why != mk.Kind || dimmed(e.From, e.To) != dim {
					continue
				}
				pa, fok := pos(e.From)
				pb, tok := pos(e.To)
				if !fok || !tok {
					continue
				}
				switch {
				case dashed:
					const dashes = 6
					for d := 0; d < dashes; d++ {
						f0 := float32(d) / dashes
						f1 := (float32(d) + 0.5) / dashes
						comp.Segment(&p, at(pa, pb, f0), at(pa, pb, f1), width*v.zoom)
					}
				case curved:
					// A straight quad can't stand in for a curve, so a solid
					// edge is walked as several short ones instead - fine
					// once a frame at the node counts this panel draws.
					const steps = 10
					for s := 0; s < steps; s++ {
						f0 := float32(s) / steps
						f1 := float32(s+1) / steps
						comp.Segment(&p, at(pa, pb, f0), at(pa, pb, f1), width*v.zoom)
					}
				default:
					comp.Segment(&p, pa, pb, width*v.zoom)
				}
				drawn++
			}
			spec := p.End()
			if drawn == 0 {
				continue
			}
			use := col
			if dim {
				use = theme.Alpha(col, dimAlpha)
			}
			paint.FillShape(gtx.Ops, use, clip.Outline{Path: spec}.Op())
		}
	}

	r := float32(gtx.Dp(4)) * clampF(v.zoom, 0.6, 1.6)
	drawDots := func(dim bool, col colorNRGBA) {
		var dots clip.Path
		dots.Begin(gtx.Ops)
		drawn := 0
		for _, n := range g.Nodes {
			if n.Name == g.Origin {
				continue
			}
			p, ok := pos(n.Name)
			if !ok {
				continue
			}
			wantDim := v.focus != "" && !focusSet[n.Name]
			if wantDim != dim {
				continue
			}
			comp.Disc(&dots, p, r)
			drawn++
		}
		spec := dots.End()
		if drawn > 0 {
			paint.FillShape(gtx.Ops, col, clip.Outline{Path: spec}.Op())
		}
	}
	drawDots(true, theme.Alpha(t.P.Ink, dimAlpha))
	drawDots(false, t.P.Ink)
	// Drawn last, on top of everything else, so the origin is never hidden
	// under an edge or a neighbouring node at the same layer.
	if p, ok := pos(g.Origin); ok {
		badCol := t.P.Bad
		if v.focus != "" && !focusSet[g.Origin] {
			badCol = theme.Alpha(badCol, dimAlpha)
		}
		var originDot clip.Path
		originDot.Begin(gtx.Ops)
		comp.Disc(&originDot, p, r)
		paint.FillShape(gtx.Ops, badCol, clip.Outline{Path: originDot.End()}.Op())
	}

	drawGraphLabels(t, gtx, g, v, pos, w, h, focusSet)
	return layout.Dimensions{Size: sz}
}

// drawGraphLabels places a name beside as many nodes as fit without their
// boxes overlapping, greedily, in priority order: the origin always first,
// then - if a click is focusing one node - only that neighbourhood, or
// otherwise every node in the graph's own order. A fixed zoom or node-count
// threshold either showed a smear of overlapping text or hid labels a
// spacious picture had plenty of room for; measuring actual overlap is what
// both of those were trying to approximate.
func drawGraphLabels(t *theme.Theme, gtx layout.Context, g hopGraph, v *graphView,
	pos func(string) (f32.Point, bool), w, h int, focusSet map[string]bool) {
	var order []string
	seen := map[string]bool{}
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		order = append(order, name)
	}
	add(g.Origin)
	// Whatever the pointer is on, and whichever edge is pinned, before the
	// rest - a name asked for by pointing is the one name somebody wants.
	add(v.hoverNode)
	add(v.pinFrom)
	add(v.pinTo)
	if v.focus != "" {
		for _, n := range g.Nodes {
			if focusSet[n.Name] {
				add(n.Name)
			}
		}
	} else {
		for _, n := range g.Nodes {
			add(n.Name)
		}
	}

	placedName := map[string]bool{}
	type box struct{ x0, y0, x1, y1 float32 }
	overlaps := func(a, b box) bool {
		return a.x0 < b.x1 && a.x1 > b.x0 && a.y0 < b.y1 && a.y1 > b.y0
	}
	lw, lh := float32(gtx.Dp(labelWidth)), float32(gtx.Dp(16))
	offX, offY := float32(gtx.Dp(7)), float32(gtx.Dp(6))
	var placed []box
	for _, name := range order {
		p, ok := pos(name)
		if !ok {
			continue
		}
		if p.X < -20 || p.X > float32(w) || p.Y < -10 || p.Y > float32(h) {
			continue
		}
		b := box{p.X + offX, p.Y - offY, p.X + offX + lw, p.Y - offY + lh}
		fits := true
		for _, other := range placed {
			if overlaps(b, other) {
				fits = false
				break
			}
		}
		if !fits {
			continue
		}
		placed = append(placed, b)
		placedName[name] = true

		drawOneLabel(t, gtx, name, p, g.Origin)
	}

	// The hovered node's label is drawn whatever the crowd, on top of
	// everything and over its own plate so it is readable against the lines
	// it necessarily overlaps. Without this a dense layer answers "what is
	// that node" with silence, and the only way through was to zoom.
	if v.hoverNode != "" && !placedName[v.hoverNode] {
		if p, ok := pos(v.hoverNode); ok {
			drawLabelPlate(t, gtx, v.hoverNode, p)
			drawOneLabel(t, gtx, v.hoverNode, p, g.Origin)
		}
	}
}

// drawOneLabel puts a name beside its node.
func drawOneLabel(t *theme.Theme, gtx layout.Context, name string, p f32.Point, origin string) {
	off := op.Offset(image.Pt(int(p.X)+gtx.Dp(7), int(p.Y)-gtx.Dp(6))).Push(gtx.Ops)
	gtx2 := gtx
	gtx2.Constraints.Max.X = gtx.Dp(labelWidth)
	ink := t.P.Dim
	if name == origin {
		ink = t.P.Bad
	}
	comp.OneLine(t, t.Sz.Caption, ink, shortName(name), false)(gtx2)
	off.Pop()
}

// drawLabelPlate is the backing a forced label sits on, so it reads against
// whatever it has landed over.
func drawLabelPlate(t *theme.Theme, gtx layout.Context, name string, p f32.Point) {
	w := gtx.Dp(unitDp(len(shortName(name))*7 + 8))
	h := gtx.Dp(18)
	off := op.Offset(image.Pt(int(p.X)+gtx.Dp(4), int(p.Y)-gtx.Dp(9))).Push(gtx.Ops)
	comp.RoundRect(gtx, image.Pt(w, h), 4, theme.Alpha(t.P.Ground, 0.92))
	off.Pop()
}

// lerp is a point a fraction of the way along a line.
func lerp(a, b f32.Point, f float32) f32.Point {
	return f32.Pt(a.X+(b.X-a.X)*f, a.Y+(b.Y-a.Y)*f)
}

// quadAt is a point a fraction of the way along a quadratic Bezier from a to
// b, bowed through ctrl.
func quadAt(a, ctrl, b f32.Point, f float32) f32.Point {
	u := 1 - f
	x := u*u*a.X + 2*u*f*ctrl.X + f*f*b.X
	y := u*u*a.Y + 2*u*f*ctrl.Y + f*f*b.Y
	return f32.Pt(x, y)
}

// curveEdge is a point a fraction of the way from a to b - a straight lerp,
// or bowed 18% of the way toward centre when curved is true. The one place
// this decision is made, so hitEdge's hit-testing walks exactly the curve
// drawHopGraph is about to paint rather than a close approximation of it that
// could drift and put a hover a few pixels from what it looks like it hit.
func curveEdge(curved bool, center, a, b f32.Point, f float32) f32.Point {
	if !curved {
		return lerp(a, b, f)
	}
	mid := lerp(a, b, 0.5)
	ctrl := lerp(mid, center, 0.18)
	return quadAt(a, ctrl, b, f)
}

// ringSides is how many straight segments approximate one ring guide. Cheap
// at the handful of rings a journey ever has, and smooth enough that nobody
// would count the corners.
const ringSides = 72

// drawRadialRings paints a faint circle at each hop's radius, so the radial
// layout reads as rings-from-an-origin on sight rather than asking the eye to
// infer distance from a scatter of dots. Built in the layout's own logical
// space using the same radialSpacing the nodes were placed with, then scaled
// and panned exactly as pos() transforms a node - a ring drawn in screen
// space some other way would drift from its nodes the moment either zoom or
// pan changed.
func drawRadialRings(t *theme.Theme, gtx layout.Context, g hopGraph, v *graphView, w, h int) {
	if g.Layers <= 1 {
		return
	}
	spacing := radialSpacing(g, w, h, float32(gtx.Dp(28)))
	cx := float32(w)/2*v.zoom + v.pan.X
	cy := float32(h)/2*v.zoom + v.pan.Y
	const width = 1
	var p clip.Path
	p.Begin(gtx.Ops)
	for l := 1; l < g.Layers; l++ {
		ringOutline(&p, f32.Pt(cx, cy), float32(l)*spacing*v.zoom, width, ringSides)
	}
	spec := p.End()
	paint.FillShape(gtx.Ops, theme.Alpha(t.P.Rule, 0.7), clip.Outline{Path: spec}.Op())
}

// ringOutline adds a thin ring - an outer polygon with an inner one cut from
// it, the same hole-punching trick comp.dotReversed uses for a filled disc -
// rather than a stroke, so this stays in the same cheap-fill idiom as every
// other shape this panel draws instead of pulling in Gio's general stroke
// machinery for one more shape.
func ringOutline(p *clip.Path, c f32.Point, r, width float32, sides int) {
	if r <= width {
		return
	}
	outer, inner := r+width/2, r-width/2
	for i := 0; i <= sides; i++ {
		a := float64(i) / float64(sides) * 2 * math.Pi
		pt := f32.Pt(c.X+float32(math.Cos(a))*outer, c.Y+float32(math.Sin(a))*outer)
		if i == 0 {
			p.MoveTo(pt)
		} else {
			p.LineTo(pt)
		}
	}
	p.Close()
	for i := sides; i >= 0; i-- {
		a := float64(i) / float64(sides) * 2 * math.Pi
		pt := f32.Pt(c.X+float32(math.Cos(a))*inner, c.Y+float32(math.Sin(a))*inner)
		if i == sides {
			p.MoveTo(pt)
		} else {
			p.LineTo(pt)
		}
	}
	p.Close()
}

// shortName keeps a label from swallowing its neighbours.
func shortName(s string) string {
	const max = 16
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
