// Three ways to place the same hopGraph's nodes in the panel's own pixel
// space, before drawHopGraph applies pan and zoom on top. Every layout
// returns positions for exactly the nodes g.Nodes names - drawHopGraph skips
// an edge whose endpoint is missing rather than assume every layout places
// everything.
package packetview

import (
	"math"
	"sort"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
)

// forceSignature fingerprints which nodes the graph holds, not how many.
//
// Counting was not enough. A live packet's graph is rebuilt as the flood
// spreads, and the per-layer cap can swap one node for another while the
// totals stay put - at which point the settled positions were kept, and the
// new node, having no entry in them, was silently not drawn at all, along
// with every edge touching it. The node it replaced kept its stale position
// and stayed clickable: clicking bare canvas focused a node that was no
// longer in the graph, and the whole picture dimmed with nothing to undim it.
//
// FNV-1a over the names, order-sensitive - buildHopGraph emits them in a
// settled order, so a reordering is a real change to the picture too.
func forceSignature(g hopGraph) uint64 {
	const (
		offset = 1469598103934665603
		prime  = 1099511628211
	)
	h := uint64(offset)
	for _, n := range g.Nodes {
		for i := 0; i < len(n.Name); i++ {
			h = (h ^ uint64(n.Name[i])) * prime
		}
		h = (h ^ '\n') * prime
	}
	return h*prime + uint64(len(g.Edges))
}

// positionsFor dispatches to the view's chosen layout. Radial and columns are
// cheap enough to recompute from scratch every frame. Free-form is a standing
// physics simulation instead - reseeded with a full settle only when the
// graph's own shape changes, then nudged one small step further every frame
// after that, so a converged layout keeps a little life to it rather than
// sitting dead still, and a dragged node pushes its neighbours as it moves
// instead of the whole picture snapping back the moment it is released.
func positionsFor(gtx layout.Context, v *graphView, g hopGraph, w, h int) map[string]f32.Point {
	switch v.mode {
	case modeRadial:
		return layoutRadial(gtx, g, w, h)
	case modeForce:
		if v.forcePos == nil || v.forceSig != forceSignature(g) {
			v.forcePos, v.forceVel = seedForce(g, w, h)
			v.forceSig = forceSignature(g)
		}
		stepForce(g, v.forcePos, v.forceVel, w, h, v.dragNode, forceWiggle)
		return v.forcePos
	default:
		return layoutColumns(gtx, g, w, h)
	}
}

// layoutColumns is today's layout: one column per hop, rows in the order
// buildHopGraph already worked out (first-seen, refined by a barycenter
// sweep so a row tracks what it actually connects to).
func layoutColumns(gtx layout.Context, g hopGraph, w, h int) map[string]f32.Point {
	out := make(map[string]f32.Point, len(g.Nodes))
	// Asymmetric padding on purpose. Labels are drawn to the right of their
	// node, so the last layer needs a label's width of room or its names
	// run off the panel.
	padL, padY := float32(gtx.Dp(16)), float32(gtx.Dp(18))
	padR := float32(gtx.Dp(labelWidth + 12))
	stepX := float32(gtx.Dp(120))
	if g.Layers > 1 {
		// Fit by default, but never squeeze layers closer than a label
		// needs; past that the operator zooms instead.
		fit := (float32(w) - padL - padR) / float32(g.Layers-1)
		if fit > stepX {
			stepX = fit
		}
	}
	stepY := float32(gtx.Dp(22))
	if g.Wide > 1 {
		fit := (float32(h) - 2*padY) / float32(g.Wide-1)
		if fit < stepY {
			stepY = fit
		}
	}
	rowsIn := map[int]int{}
	for _, n := range g.Nodes {
		rowsIn[n.Layer]++
	}
	for _, n := range g.Nodes {
		x := padL + float32(n.Layer)*stepX
		off := (float32(g.Wide-rowsIn[n.Layer]) / 2) * stepY
		y := padY + off + float32(n.Row)*stepY
		if g.Wide == 1 {
			y = float32(h) / 2
		}
		out[n.Name] = f32.Pt(x, y)
	}
	return out
}

// layoutRadial rings the journey out from its origin: hop N sits on ring N,
// and a node's angle is the mean angle of whichever of its parents actually
// heard it, so a branch stays a wedge rather than scattering around the
// circle. The one thing every journey shares - distance from the origin -
// becomes the one thing the eye reads first.
func layoutRadial(gtx layout.Context, g hopGraph, w, h int) map[string]f32.Point {
	out := make(map[string]f32.Point, len(g.Nodes))
	if len(g.Nodes) == 0 {
		return out
	}
	cx, cy := float32(w)/2, float32(h)/2
	// A circle, not an ellipse: a ring is the one shape an operator recognises
	// on sight without reading a label, and an ellipse stretched to a docked
	// panel's aspect ratio stops reading as "distance from the origin" and
	// starts reading as an unexplained tangle. drawHopGraph's own
	// fit-to-content zoom already fills the panel afterwards, so there is
	// nothing an ellipse would have bought here.
	spacing := radialSpacing(g, w, h, float32(gtx.Dp(28)))

	byLayer := map[int][]hopNode{}
	for _, n := range g.Nodes {
		byLayer[n.Layer] = append(byLayer[n.Layer], n)
	}

	angle := make(map[string]float64, len(g.Nodes))
	for _, n := range byLayer[0] {
		angle[n.Name] = 0
		out[n.Name] = f32.Pt(cx, cy)
	}
	type placed struct {
		node hopNode
		ang  float64
	}
	for l := 1; l < g.Layers; l++ {
		layer := byLayer[l]
		ordered := make([]placed, len(layer))
		for i, n := range layer {
			sum, count := 0.0, 0
			for _, e := range g.Edges {
				if e.To == n.Name && e.Why == missNone {
					if a, ok := angle[e.From]; ok {
						sum += a
						count++
					}
				}
			}
			if count > 0 {
				ordered[i] = placed{n, sum / float64(count)}
			} else {
				// Nothing heard it directly (a hop limit or a filtered
				// reason can do that): fall back to its already-settled row
				// order, which is at least stable between frames.
				ordered[i] = placed{n, float64(n.Row)}
			}
		}
		sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].ang < ordered[j].ang })
		r := float32(l) * spacing
		for i, p := range ordered {
			a := (float64(i) / float64(len(ordered))) * 2 * math.Pi
			angle[p.node.Name] = a
			out[p.node.Name] = f32.Pt(
				cx+float32(math.Cos(a-math.Pi/2))*r,
				cy+float32(math.Sin(a-math.Pi/2))*r,
			)
		}
	}
	return out
}

// radialSpacing is the distance from one hop's ring to the next. Shared
// between layoutRadial, which places nodes on it, and the faint ring guides
// drawHopGraph paints under them - computed once here so the two can never
// drift apart and draw a node that sits visibly off its own ring.
func radialSpacing(g hopGraph, w, h int, minRing float32) float32 {
	rings := float32(g.Layers - 1)
	if rings < 1 {
		rings = 1
	}
	return maxF(minF(float32(w), float32(h))/2/(rings+1.6), minRing)
}

// forceRepel, forceRest and forceDamping are the free-form layout's physics:
// every node repels every other, every edge pulls like a spring at forceRest
// long, and forceDamping bleeds off velocity so the system settles instead of
// oscillating forever. What comes out is the mesh's own shape - clusters of
// nodes that talk to each other end up near each other - which a fixed
// hop-column can never show, because it spends the entire cross-axis on
// "which hop" and none of it on "who is actually near whom."
// forceRepel and forceRest are deliberately not scaled up together. The view
// auto-fits the settled shape to the panel afterwards (fitPositions), so a
// uniform scale-up of both would settle to a physically bigger picture that
// zooms right back down to the same on-screen density - no more legible than
// before. What actually spreads the shape out, once the fit rescales it away,
// is the *ratio* between them: a longer rest length pulls connected nodes
// further apart relative to how tightly repulsion alone clusters unconnected
// ones, stretching the mesh along its own edges rather than just its scale.
const (
	forceRepel   = 3200.0
	forceRest    = 100.0
	forceDamping = 0.8
)

// forceWiggle is how much the settled free-form layout keeps moving on its
// own, in pixels of oscillating force per frame. Big enough to read as
// "alive" without anyone having to be told to look for it; small enough that
// nobody mistakes it for the graph still settling, or for a bug.
const forceWiggle = 3.5

// seedForce places every node on a circle, wider rings further out by hop -
// deterministic, not random, so the same journey opens to the same starting
// shape every time - then runs a full settle: several hundred
// spring-and-repulsion iterations, the expensive part, so a freshly opened
// packet starts from a good shape rather than the raw seed. Velocities come
// back alongside positions so stepForce can carry on from exactly where this
// left off, one small nudge at a time, instead of re-settling from scratch
// every frame.
func seedForce(g hopGraph, w, h int) (pos, vel map[string]f32.Point) {
	n := len(g.Nodes)
	pos = make(map[string]f32.Point, n)
	vel = make(map[string]f32.Point, n)
	if n == 0 {
		return pos, vel
	}
	cx, cy := float32(w)/2, float32(h)/2
	for i, nd := range g.Nodes {
		a := (float64(i) / float64(n)) * 2 * math.Pi
		r := float32(30 + nd.Layer*45)
		pos[nd.Name] = f32.Pt(cx+float32(math.Cos(a))*r, cy+float32(math.Sin(a))*r)
	}
	const settleIterations = 260
	for i := 0; i < settleIterations; i++ {
		stepForce(g, pos, vel, w, h, "", 0)
	}
	return pos, vel
}

// stepForce advances the free-form layout by one iteration: repulsion
// between every pair, springs along every edge, a weak pull to centre and -
// when wiggle is nonzero - a slow per-node oscillation, each node out of
// phase with the next so the picture breathes rather than jumping as one
// block. pinned is excluded from every force but still feels its neighbours'
// pull on them: dragging a node has to feel like moving it, not fighting the
// simulation for control of where it is.
func stepForce(g hopGraph, pos, vel map[string]f32.Point, w, h int, pinned string, wiggle float32) {
	cx, cy := float32(w)/2, float32(h)/2
	t := float32(time.Now().UnixMilli()) / 1000
	for i, ni := range g.Nodes {
		if ni.Name == pinned {
			continue
		}
		a, ok := pos[ni.Name]
		if !ok {
			continue
		}
		var fx, fy float32
		for _, nj := range g.Nodes {
			if nj.Name == ni.Name {
				continue
			}
			b, ok := pos[nj.Name]
			if !ok {
				continue
			}
			dx, dy := a.X-b.X, a.Y-b.Y
			d2 := dx*dx + dy*dy + 0.01
			d := float32(math.Sqrt(float64(d2)))
			f := float32(forceRepel) / d2
			fx += dx / d * f
			fy += dy / d * f
		}
		// A weak pull toward the centre - just enough that a graph with no
		// edges at all (a hop limit of zero edges left) does not fly apart,
		// not enough to fight the springs for shape.
		fx += (cx - a.X) * 0.01
		fy += (cy - a.Y) * 0.01
		if wiggle != 0 {
			phase := float64(i) * 0.73
			fx += wiggle * float32(math.Cos(float64(t)*0.6+phase))
			fy += wiggle * float32(math.Sin(float64(t)*0.6+phase*1.3))
		}
		v := vel[ni.Name]
		v.X = (v.X + fx*0.02) * forceDamping
		v.Y = (v.Y + fy*0.02) * forceDamping
		vel[ni.Name] = v
	}
	for _, e := range g.Edges {
		a, aok := pos[e.From]
		b, bok := pos[e.To]
		if !aok || !bok {
			continue
		}
		dx, dy := b.X-a.X, b.Y-a.Y
		d := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if d == 0 {
			d = 0.01
		}
		f := (d - forceRest) * 0.02
		ux, uy := dx/d, dy/d
		if e.From != pinned {
			va := vel[e.From]
			va.X += ux * f
			va.Y += uy * f
			vel[e.From] = va
		}
		if e.To != pinned {
			vb := vel[e.To]
			vb.X -= ux * f
			vb.Y -= uy * f
			vel[e.To] = vb
		}
	}
	for _, nd := range g.Nodes {
		if nd.Name == pinned {
			continue
		}
		p, ok := pos[nd.Name]
		if !ok {
			continue
		}
		v := vel[nd.Name]
		p.X += v.X
		p.Y += v.Y
		pos[nd.Name] = p
	}
}
