package workbench

import (
	"fmt"
	"image"
	"sort"

	"image/color"
	"strings"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// A packet's propagation, drawn — including the hops that failed.
//
// The Journey tab has always had this data and rendered it as a list of rows:
// at this time, this node relayed, and these heard it, and these did not. That
// answers what happened and not what shape it was, and the shape is what
// somebody chasing a packet is usually after.
//
// The reason to draw it here rather than take CoreScope's picture is that we
// know more than CoreScope can. It reconstructs paths from what observers
// happened to overhear, so its graph is honestly a sample of the successes.
// This ran the channel, so it also knows every reception that *failed*, and
// those edges are the ones that explain a packet that did not arrive.

// hopEdge is one node hearing, or failing to hear, one transmission.
type hopEdge struct {
	From, To string
	AtMs     uint32
	// Hop is the transmission's own hop count, off the frame.
	Hop int
	// Why is what became of it. A failed edge is not an absence: something
	// arrived and was not demodulated, and *which* failure it was decides what
	// an operator would do about it. Too weak wants a better antenna; lost to
	// an interferer wants less traffic; deaf wants different timing. Drawing
	// all three the same colour throws that away.
	Why missKind
}

// OK reports a reception that succeeded.
func (e hopEdge) OK() bool { return e.Why == missNone }

// missKind is why a reception did not happen, in the categories the engine
// already distinguishes when it records the miss.
type missKind int

const (
	missNone missKind = iota
	missWeak
	missCollided
	missDeaf
	missOther
)

// missKinds is the legend, in the order it is drawn.
var missKinds = []struct {
	Kind  missKind
	Label string
}{
	{missNone, "heard"},
	{missWeak, "too weak"},
	{missCollided, "lost to a stronger signal"},
	{missDeaf, "deaf — was transmitting"},
	{missOther, "other"},
}

// colourOf is the one place a reason becomes a colour, so the key and the
// drawing cannot disagree.
func colourOf(t *theme.Theme, k missKind) (col color.NRGBA, width float32, dashed bool) {
	switch k {
	case missNone:
		return theme.Alpha(t.P.Accent, 0.75), 1.6, false
	case missWeak:
		return theme.Alpha(t.P.Bad, 0.6), 1.0, false
	case missCollided:
		return theme.Alpha(t.P.Warn, 0.7), 1.0, false
	case missDeaf:
		return theme.Alpha(t.P.Warn, 0.75), 1.0, true
	}
	return theme.Alpha(t.P.Faint, 0.6), 0.8, true
}

// classifyMiss reads the engine's own words for a miss.
//
// The strings are the ones recorded in engine.deliver, and they are matched
// rather than re-derived so the picture cannot disagree with the ledger row
// beside it.
func classifyMiss(what string) missKind {
	switch {
	case strings.Contains(what, "stronger interferer"):
		return missCollided
	case strings.Contains(what, "half duplex"), strings.Contains(what, "own transmitter"):
		return missDeaf
	case strings.Contains(what, "needed"):
		return missWeak
	}
	return missOther
}

// hopNode is a radio, placed.
type hopNode struct {
	Name  string
	Layer int
	Row   int
}

// hopGraph is the whole propagation, laid out.
type hopGraph struct {
	Nodes []hopNode
	Edges []hopEdge
	// Layers is how deep the drawing goes; Wide is the most nodes in any one
	// layer. Deepest is how far the packet actually travelled, which is not
	// the same thing once a hop limit is applied.
	Layers, Wide, Deepest int
	// Dropped is edges left out to keep the drawing legible, Hidden is nodes,
	// and Beyond is what a hop limit excluded. All three are displayed: a
	// truncated graph that looks complete is worse than a small one that
	// admits it.
	Dropped, Hidden, Beyond int
	// Total counts every edge by reason before any filter, so the key can show
	// what hiding a class costs.
	Total map[missKind]int
}

// maxGraphEdges is where legibility gives out. Chosen by looking: beyond this
// the picture is a solid block whatever the layout does.
const maxGraphEdges = 400

// maxRowsPerLayer is how many nodes one hop can show. An advert heard by
// twenty-five neighbours is a fan, not a shape, and drawing all of it turns
// the panel into hatching. Beyond this the layer is summarised.
const maxRowsPerLayer = 12

// buildHopGraph turns a packet's journey into a laid-out graph.
//
// Layering is the packet's own hop count, taken off the frame at each
// transmission, not a breadth-first distance through the graph. Those are
// different numbers and the firmware's one is the right one: a node that hears
// the origin directly and again at hop three is at both depths, and the flood
// really did reach hop three. The first version used breadth-first distance
// and collapsed a five-hop flood into two layers.
//
// maxHops caps the drawing at that many hops; zero draws all of them. show
// selects which reasons to draw, so an operator chasing collisions can hide the
// links that were simply out of budget.
func buildHopGraph(pk *state.Packet, maxHops int, show map[missKind]bool) hopGraph {
	g := hopGraph{Total: map[missKind]int{}}
	if pk == nil {
		return g
	}
	origin, hops := pk.Origin, pk.Journey

	drawn := func(k missKind) bool { return show == nil || show[k] }

	// The hop each node is first seen at. A transmitter sits at its frame's
	// hop count; anyone who hears it sits one further out.
	layer := map[string]int{}
	place := func(name string, at int) {
		if name == "" {
			return
		}
		if cur, seen := layer[name]; !seen || at < cur {
			layer[name] = at
		}
	}
	for _, h := range hops {
		place(h.By, h.Hops)
		for _, n := range h.Heard {
			place(n, h.Hops+1)
		}
		for _, n := range h.MissedBy {
			place(n, h.Hops+1)
		}
		if h.Hops > g.Deepest {
			g.Deepest = h.Hops
		}
	}
	if len(layer) == 0 {
		return g
	}
	// The origin anchors the left edge even when its own frame says otherwise.
	if _, ok := layer[origin]; ok {
		layer[origin] = 0
	}

	within := func(name string) bool {
		return maxHops <= 0 || layer[name] <= maxHops
	}
	add := func(from, to string, at uint32, hop int, k missKind) {
		if from == "" || to == "" || from == to {
			return
		}
		if !within(from) || !within(to) {
			g.Beyond++
			return
		}
		g.Total[k]++
		if !drawn(k) {
			return
		}
		if len(g.Edges) >= maxGraphEdges {
			g.Dropped++
			return
		}
		g.Edges = append(g.Edges, hopEdge{From: from, To: to, AtMs: at, Hop: hop, Why: k})
	}
	for _, h := range hops {
		for _, n := range h.Heard {
			add(h.By, n, h.AtMs, h.Hops, missNone)
		}
		for i, n := range h.MissedBy {
			// The engine's own words for this exact miss, carried on the row
			// rather than looked up: a journey spans every transmission of the
			// message and Fates covers only one packet of it.
			what := ""
			if i < len(h.MissWhy) {
				what = h.MissWhy[i]
			}
			add(h.By, n, h.AtMs, h.Hops, classifyMiss(what))
		}
	}

	// Rows within a layer, in first-seen order so the picture is stable
	// between frames and between runs.
	order := map[string]int{}
	seq := 0
	note := func(n string) {
		if n == "" {
			return
		}
		if _, ok := order[n]; !ok {
			order[n] = seq
			seq++
		}
	}
	for _, h := range hops {
		note(h.By)
		for _, n := range h.Heard {
			note(n)
		}
		for _, n := range h.MissedBy {
			note(n)
		}
	}

	names := make([]string, 0, len(layer))
	for n := range layer {
		if within(n) {
			names = append(names, n)
		}
	}
	sort.Slice(names, func(i, j int) bool {
		if layer[names[i]] != layer[names[j]] {
			return layer[names[i]] < layer[names[j]]
		}
		return order[names[i]] < order[names[j]]
	})
	rows := map[int]int{}
	for _, n := range names {
		l := layer[n]
		if rows[l] >= maxRowsPerLayer {
			g.Hidden++
			continue
		}
		g.Nodes = append(g.Nodes, hopNode{Name: n, Layer: l, Row: rows[l]})
		rows[l]++
		if rows[l] > g.Wide {
			g.Wide = rows[l]
		}
		if l+1 > g.Layers {
			g.Layers = l + 1
		}
	}
	return g
}

// at finds a laid-out node.
func (g hopGraph) at(name string) (hopNode, bool) {
	for _, n := range g.Nodes {
		if n.Name == name {
			return n, true
		}
	}
	return hopNode{}, false
}

// counts is how it went, for the caption. Over every edge including the
// filtered-out ones: hiding a class from the picture must not change the tally
// of what happened.
func (g hopGraph) counts() (ok, failed int) {
	for k, n := range g.Total {
		if k == missNone {
			ok += n
		} else {
			failed += n
		}
	}
	return ok, failed
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
	// maxHops is how deep to draw; zero is all of it.
	maxHops int
}

const (
	minGraphZoom = 0.35
	maxGraphZoom = 6
)

// handle takes the frame's pointer events: drag to pan, wheel to zoom.
func (v *graphView) handle(gtx layout.Context, sz image.Point) {
	if v.zoom == 0 {
		v.zoom = 1
	}
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, v)
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  v,
			Kinds:   pointer.Press | pointer.Drag | pointer.Release | pointer.Scroll,
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
			v.dragging, v.last = true, e.Position
		case pointer.Drag:
			if v.dragging {
				v.pan.X += e.Position.X - v.last.X
				v.pan.Y += e.Position.Y - v.last.Y
				v.last = e.Position
			}
		case pointer.Release:
			v.dragging = false
		}
	}
}

// reset puts the picture back where it started.
func (v *graphView) reset() { v.pan, v.zoom = f32.Point{}, 1 }

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

// drawHopGraph paints the graph into the space it is given.
//
// Fixed height rather than flexed: this sits above the journey table, and a
// graph that grows until the table is off the bottom of a docked panel is
// worse than a small graph you can drag.
func drawHopGraph(t *theme.Theme, gtx layout.Context, g hopGraph, v *graphView) layout.Dimensions {
	h := gtx.Dp(graphHeight)
	w := gtx.Constraints.Max.X
	sz := image.Pt(w, h)
	if len(g.Nodes) == 0 || w <= 0 {
		return layout.Dimensions{Size: image.Pt(w, 0)}
	}
	v.handle(gtx, sz)
	if v.zoom == 0 {
		v.zoom = 1
	}
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()

	// Asymmetric on purpose. Labels are drawn to the right of their node, so
	// the last layer needs a label's width of room or its names run off the
	// panel.
	padL, padY := float32(gtx.Dp(16)), float32(gtx.Dp(18))
	padR := float32(gtx.Dp(labelWidth + 12))
	stepX := float32(gtx.Dp(120))
	if g.Layers > 1 {
		// Fit by default, but never squeeze layers closer than a label needs;
		// past that the operator drags instead.
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
	pos := func(n hopNode) f32.Point {
		x := padL + float32(n.Layer)*stepX
		off := (float32(g.Wide-rowsIn[n.Layer]) / 2) * stepY
		y := padY + off + float32(n.Row)*stepY
		if g.Wide == 1 {
			y = float32(h) / 2
		}
		return f32.Pt(x*v.zoom+v.pan.X, y*v.zoom+v.pan.Y)
	}

	// Edges first, so nodes sit on top of them, and one path per reason so
	// each gets its own colour.
	//
	// One clip.Path at a time: Gio allows only a single one open on an Ops, and
	// beginning a second before the first ends panics with "cannot mix multi
	// ops with single ones" — a crash on screen and nothing at all in a unit
	// test.
	//
	// Reversed, so the successes are painted last and sit on top of the misses.
	for i := len(missKinds) - 1; i >= 0; i-- {
		mk := missKinds[i]
		col, width, dashed := colourOf(t, mk.Kind)
		var p clip.Path
		p.Begin(gtx.Ops)
		drawn := 0
		for _, e := range g.Edges {
			if e.Why != mk.Kind {
				continue
			}
			from, fok := g.at(e.From)
			to, tok := g.at(e.To)
			if !fok || !tok {
				continue
			}
			pa, pb := pos(from), pos(to)
			if dashed {
				const dashes = 6
				for d := 0; d < dashes; d++ {
					f0 := float32(d) / dashes
					f1 := (float32(d) + 0.5) / dashes
					comp.Segment(&p, lerp(pa, pb, f0), lerp(pa, pb, f1), width*v.zoom)
				}
			} else {
				comp.Segment(&p, pa, pb, width*v.zoom)
			}
			drawn++
		}
		spec := p.End()
		if drawn > 0 {
			paint.FillShape(gtx.Ops, col, clip.Outline{Path: spec}.Op())
		}
	}

	var dots clip.Path
	dots.Begin(gtx.Ops)
	r := float32(gtx.Dp(4)) * clampF(v.zoom, 0.6, 1.6)
	for _, n := range g.Nodes {
		comp.Disc(&dots, pos(n), r)
	}
	paint.FillShape(gtx.Ops, t.P.Ink, clip.Outline{Path: dots.End()}.Op())

	// Labels, only where there is room. A layer packed with twenty nodes gets
	// dots and no names rather than a smear of overlapping text.
	if stepY*v.zoom >= float32(gtx.Dp(16)) || g.Wide == 1 {
		for _, n := range g.Nodes {
			p := pos(n)
			if p.X < -20 || p.X > float32(w) || p.Y < -10 || p.Y > float32(h) {
				continue
			}
			off := op.Offset(image.Pt(int(p.X)+gtx.Dp(7), int(p.Y)-gtx.Dp(6))).Push(gtx.Ops)
			gtx2 := gtx
			gtx2.Constraints.Max.X = gtx.Dp(labelWidth)
			comp.OneLine(t, t.Sz.Caption, t.P.Dim, shortName(n.Name), false)(gtx2)
			off.Pop()
		}
	}
	return layout.Dimensions{Size: sz}
}

// lerp is a point a fraction of the way along a line.
func lerp(a, b f32.Point, f float32) f32.Point {
	return f32.Pt(a.X+(b.X-a.X)*f, a.Y+(b.Y-a.Y)*f)
}

// labelWidth is how much room a node's name gets, and therefore how much
// space the drawing reserves on its right.
const labelWidth = 104

// graphHeight is the space the picture gets. Enough to be read, small enough
// that the table under it stays on screen in a docked panel.
const graphHeight = 210

// shortName keeps a label from swallowing its neighbours.
func shortName(s string) string {
	const max = 16
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// graphCaption says what the picture contains, including what it left out.
func graphCaption(g hopGraph) string {
	ok, failed := g.counts()
	s := fmt.Sprintf("%d relays heard, %d hops failed", ok, failed)
	if g.Deepest > 0 {
		s += fmt.Sprintf("  ·  %d hops deep", g.Deepest)
	}
	if g.Beyond > 0 {
		s += fmt.Sprintf("  ·  %d beyond the hop limit", g.Beyond)
	}
	if g.Hidden > 0 {
		s += fmt.Sprintf("  ·  %d nodes not drawn", g.Hidden)
	}
	if g.Dropped > 0 {
		s += fmt.Sprintf("  ·  %d more edges not drawn", g.Dropped)
	}
	return s
}

// hopLimits are the depths the panel offers. Zero is the whole packet, which
// is the default: a graph that silently stops at two hops is the bug this
// replaced.
var hopLimits = [5]int{0, 1, 2, 3, 4}

// hopLimitLabel names one.
func hopLimitLabel(n int) string {
	if n == 0 {
		return "all"
	}
	return fmt.Sprintf("%d", n)
}
