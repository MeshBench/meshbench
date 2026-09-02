// The Journey graph's view state: where the operator has panned and zoomed
// to, how deep they asked it to go, and which of the three shapes they are
// looking at it as.
package workbench

import (
	"image"
	"math"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
)

// graphMode is which of the three layouts is drawing the same hopGraph.
//
// Three, not a spectrum: a free-form settle, a hop-centred ring, and hop
// columns each answer a different question about the same journey, and
// blending them would answer none of the three cleanly.
type graphMode int

const (
	modeColumns graphMode = iota
	modeRadial
	modeForce
)

// graphModes is every mode, in the order the picker offers them. An array,
// so the picker's chips are declared from its length rather than beside it.
var graphModes = [...]struct {
	Mode  graphMode
	Label string
}{
	{modeColumns, "columns"},
	{modeRadial, "radial"},
	{modeForce, "free-form"},
}

// graphView is where the operator has moved and scaled the picture.
//
// A flood five hops deep does not fit a panel at a readable size, and the
// alternative to panning is shrinking it until the labels are gone. Kept on
// the panel rather than in the world: it is where somebody is looking, not
// what is true of the run.
type graphView struct {
	pan      f32.Point
	zoom     float32
	dragging bool
	last     f32.Point
	// pressAt is where the pointer went down, so a release close to it reads
	// as a click on whatever is there rather than as the end of a drag that
	// never really moved.
	pressAt f32.Point
	// maxHops is how deep to draw; zero is all of it.
	maxHops int
	// mode is which layout is drawing the graph right now.
	mode graphMode
	// fitted is whether the current pan and zoom actually frame what is
	// drawn. False schedules a fit on the next draw - set on the first frame
	// a graph exists for, and again whenever the mode, the hop depth, or the
	// "fit" button asks for one. A settle that lands as a small cluster in a
	// large empty canvas is a layout bug wearing a navigation problem's
	// clothes; framing it is cheaper than fixing every layout to fill space
	// it does not know the size of.
	fitted bool
	// focus is the node a click asked to be isolated, or empty for none.
	// Everything not connected to it dims - the tool for finding one path
	// through a mesh too dense to read as a whole.
	focus string
	// hoverFrom/hoverTo name the edge the pointer is resting on, or both
	// empty for none. A hover rather than a click because a busy graph's
	// edges overlap far more than its nodes do, and asking "which one is
	// that" is a question worth answering without spending the click that
	// focus already owns.
	hoverFrom, hoverTo string
	// pinFrom/pinTo are an edge a click asked to keep highlighted, so it
	// survives the pointer moving away - a hover answers "which line is
	// that", a click answers "keep that one while I read the table".
	pinFrom, pinTo string
	// hoverNode is the node under the pointer. A crowded layer has room for
	// far fewer labels than nodes, so most names are dropped; this is how you
	// get one back without zooming - it is drawn on top, overlap and all.
	hoverNode string
	// forcePos and forceVel are the free-form layout's own standing physics
	// simulation - positions and velocities, both persisted here rather than
	// recomputed, so stepForce can nudge them one small step further every
	// frame instead of re-settling from scratch. forceSig fingerprints which
	// nodes the graph holds, so a rebuild that swaps one node for another is
	// reseeded rather than leaving the newcomer unplaced and the departed
	// one clickable. Reseeded on a new packet, a hop-depth filter, an
	// outcome toggle - anything that changes who is in the picture.
	forcePos map[string]f32.Point
	forceVel map[string]f32.Point
	forceSig uint64
	// dragNode is the node a drag is currently repositioning, in free-form
	// mode; empty means the drag is panning the view instead, same as every
	// other mode.
	dragNode string
}

const (
	minGraphZoom = 0.35
	maxGraphZoom = 6
)

// handle takes the frame's pointer events: drag to pan, wheel to zoom, a
// click that barely moved to focus whatever node is under it, and a hover to
// highlight whatever edge is under it. positions is this frame's
// already-computed layout, in the same pre-zoom space drawing uses, so a
// click or a hover can be tested against where things actually are rather
// than where some other frame's layout put them. g is only read for its
// edges, to hit-test the same curves drawHopGraph is about to paint.
func (v *graphView) handle(gtx layout.Context, sz image.Point, g hopGraph, positions map[string]f32.Point) {
	if v.zoom == 0 {
		v.zoom = 1
	}
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, v)
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  v,
			Kinds:   pointer.Press | pointer.Drag | pointer.Release | pointer.Scroll | pointer.Move | pointer.Leave | pointer.Cancel,
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
		case pointer.Move:
			if !v.dragging {
				// The node wins: it is the smaller target, and an edge always
				// passes under the dot it ends at.
				v.hoverNode = hitNode(g, positions, v, e.Position)
				v.hoverFrom, v.hoverTo = "", ""
				if v.hoverNode == "" {
					v.hoverFrom, v.hoverTo = hitEdge(g, positions, v, sz, e.Position)
				}
			}
		case pointer.Leave, pointer.Cancel:
			v.hoverFrom, v.hoverTo, v.hoverNode = "", "", ""
		case pointer.Scroll:
			// Zoom about the pointer, not the centre: the node under it is
			// the one being looked at and should stay where it is.
			old := v.zoom
			v.zoom = clampF(v.zoom*pow11(-e.Scroll.Y), minGraphZoom, maxGraphZoom)
			if v.zoom != old {
				k := v.zoom / old
				v.pan.X = e.Position.X - k*(e.Position.X-v.pan.X)
				v.pan.Y = e.Position.Y - k*(e.Position.Y-v.pan.Y)
			}
		case pointer.Press:
			v.dragging, v.last, v.pressAt = true, e.Position, e.Position
			v.hoverFrom, v.hoverTo, v.hoverNode = "", "", ""
			// Free-form only: everywhere else a node's position is what its
			// hop says it has to be, so there is nothing dragging it could
			// mean. A hand that starts on a node moves the node; a hand that
			// starts on empty canvas pans the view, same as every mode.
			if v.mode == modeForce {
				v.dragNode = hitNode(g, positions, v, e.Position)
			}
		case pointer.Drag:
			if v.dragNode != "" {
				// Screen delta back to the layout's own pre-zoom space -
				// positions live there, and moving a node by the raw screen
				// delta would outrun the pointer at any zoom past 1x.
				p := positions[v.dragNode]
				p.X += (e.Position.X - v.last.X) / v.zoom
				p.Y += (e.Position.Y - v.last.Y) / v.zoom
				positions[v.dragNode] = p
				v.forceVel[v.dragNode] = f32.Point{}
				v.last = e.Position
			} else if v.dragging {
				v.pan.X += e.Position.X - v.last.X
				v.pan.Y += e.Position.Y - v.last.Y
				v.last = e.Position
			}
		case pointer.Release:
			v.dragging = false
			v.dragNode = ""
			// A drag that never really moved is a click. Six pixels because
			// a hand is not a mouse-precise instrument, and a click that
			// missed by that little should still land on the node it was
			// obviously aimed at.
			const clickSlop = 6
			if hyp(e.Position.X-v.pressAt.X, e.Position.Y-v.pressAt.Y) <= clickSlop {
				if hit := hitNode(g, positions, v, e.Position); hit != "" {
					v.pinFrom, v.pinTo = "", ""
					if hit == v.focus {
						v.focus = ""
					} else {
						v.focus = hit
					}
					break
				}
				// Nothing under the pointer at node range: an edge, then, and
				// failing that a click on empty canvas, which clears both.
				from, to := hitEdge(g, positions, v, sz, e.Position)
				switch {
				case from == "":
					v.focus, v.pinFrom, v.pinTo = "", "", ""
				case from == v.pinFrom && to == v.pinTo:
					v.pinFrom, v.pinTo = "", ""
				default:
					v.focus, v.pinFrom, v.pinTo = "", from, to
				}
			}
		}
	}
}

// hitNode is which node, if any, a screen point landed on - checked against
// this frame's positions transformed by the view's own pan and zoom, so a
// click is tested against exactly what is on screen right now.
func hitNode(g hopGraph, positions map[string]f32.Point, v *graphView, at f32.Point) string {
	const radius = 11.0
	best, bestD2 := "", float32(radius*radius)
	for _, n := range g.Nodes {
		name := n.Name
		p, ok := positions[name]
		if !ok {
			continue
		}
		sx, sy := p.X*v.zoom+v.pan.X, p.Y*v.zoom+v.pan.Y
		dx, dy := sx-at.X, sy-at.Y
		d2 := dx*dx + dy*dy
		if d2 <= bestD2 {
			best, bestD2 = name, d2
		}
	}
	return best
}

// hitEdge is which edge, if any, a screen point landed near - walked as short
// straight segments along the same curve drawHopGraph paints (curveEdge, the
// one place that decision is made), so a hover lands on what the eye is
// actually looking at rather than on the straight chord a curved edge no
// longer follows. Positions are pre-zoom, transformed here exactly as
// hitNode transforms them, so a hover is tested against exactly what is on
// screen right now.
func hitEdge(g hopGraph, positions map[string]f32.Point, v *graphView, sz image.Point, at f32.Point) (from, to string) {
	const radius = 6.0
	bestD2 := float32(radius * radius)
	curved := v.mode == modeRadial
	center := f32.Pt(float32(sz.X)/2*v.zoom+v.pan.X, float32(sz.Y)/2*v.zoom+v.pan.Y)
	toScreen := func(p f32.Point) f32.Point {
		return f32.Pt(p.X*v.zoom+v.pan.X, p.Y*v.zoom+v.pan.Y)
	}
	const steps = 10
	for _, e := range g.Edges {
		la, aok := positions[e.From]
		lb, bok := positions[e.To]
		if !aok || !bok {
			continue
		}
		pa, pb := toScreen(la), toScreen(lb)
		prev := curveEdge(curved, center, pa, pb, 0)
		for s := 1; s <= steps; s++ {
			cur := curveEdge(curved, center, pa, pb, float32(s)/steps)
			if d2 := distToSegSq(at, prev, cur); d2 <= bestD2 {
				bestD2, from, to = d2, e.From, e.To
			}
			prev = cur
		}
	}
	return from, to
}

// distToSegSq is the squared distance from p to the segment ab.
func distToSegSq(p, a, b f32.Point) float32 {
	abx, aby := b.X-a.X, b.Y-a.Y
	l2 := abx*abx + aby*aby
	if l2 == 0 {
		dx, dy := p.X-a.X, p.Y-a.Y
		return dx*dx + dy*dy
	}
	t := clampF(((p.X-a.X)*abx+(p.Y-a.Y)*aby)/l2, 0, 1)
	cx, cy := a.X+t*abx, a.Y+t*aby
	dx, dy := p.X-cx, p.Y-cy
	return dx*dx + dy*dy
}

func hyp(dx, dy float32) float32 {
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}

// reset asks the next draw to fit the picture to what it actually contains.
// The button beside it is labelled "fit" - this is what makes that true,
// rather than the flat zoom-of-one it used to fall back to, which left a
// small cluster looking lost in the middle of a large empty canvas.
func (v *graphView) reset() { v.fitted = false }

// fitPositions frames every position with a margin, choosing the largest
// zoom that still keeps all of them on screen. Clamped well below what an
// unzoomed single-node graph would otherwise compute, so opening a packet
// with almost nothing in it does not fill the panel with one giant dot.
func fitPositions(pos map[string]f32.Point, w, h int) (f32.Point, float32) {
	if len(pos) == 0 {
		return f32.Point{}, 1
	}
	var minX, minY, maxX, maxY float32
	first := true
	for _, p := range pos {
		if first {
			minX, maxX, minY, maxY = p.X, p.X, p.Y, p.Y
			first = false
			continue
		}
		minX, maxX = minF(minX, p.X), maxF(maxX, p.X)
		minY, maxY = minF(minY, p.Y), maxF(maxY, p.Y)
	}
	cw, ch := maxX-minX, maxY-minY
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}
	const margin = 36
	zoom := minF((float32(w)-margin*2)/cw, (float32(h)-margin*2)/ch)
	zoom = clampF(zoom, minGraphZoom, 2.2)
	cx, cy := (minX+maxX)/2, (minY+maxY)/2
	return f32.Pt(float32(w)/2-cx*zoom, float32(h)/2-cy*zoom), zoom
}

func minF(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func maxF(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func clampF(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// pow11 is 1.1^n without importing math for one call.
func pow11(n float32) float32 {
	p := float32(1)
	for i := 0; i < 8 && n > 0; i++ {
		p *= 1.1
		n--
	}
	for i := 0; i < 8 && n < 0; i++ {
		p /= 1.1
		n++
	}
	return p
}
