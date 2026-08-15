package workbench

import (
	"fmt"
	"image"
	"sort"

	"gioui.org/f32"
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
// answers "what happened" and not "what shape was it", and the shape is what
// somebody looking at a packet is usually after.
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
	// OK is whether the receiver decoded it. A failed edge is not an absence:
	// something arrived and was not demodulated, which is a different fact
	// from nothing arriving at all.
	OK bool
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
	// Layers is how deep it got; Wide is the most nodes in any one layer.
	Layers, Wide int
	// Dropped is how many edges were left out to keep the drawing legible,
	// and is displayed. A country-sized flood has hundreds of edges and no
	// useful picture; silently truncating it would make a partial graph look
	// like a complete one.
	Dropped int
	// Hidden is nodes left out of crowded layers, for the same reason.
	Hidden int
}

// maxGraphEdges is where legibility gives out. Chosen by looking: beyond this
// the picture is a solid block whatever the layout does.
const maxGraphEdges = 120

// maxRowsPerLayer is how many nodes one hop can show. An advert heard by
// twenty-five neighbours is a fan, not a shape, and drawing all of it turns
// the panel into hatching - which is what the first version did. Beyond this
// the layer is summarised and the caption says how many were left out.
const maxRowsPerLayer = 8

// buildHopGraph turns a packet's journey into a laid-out graph.
//
// Layering is breadth-first from the origin over every edge, successful or
// not, because a node that failed to hear a relay is still one hop further out
// than the node that sent it. Breadth-first rather than longest-path so a mesh
// that loops — and they do — cannot recurse.
func buildHopGraph(origin string, hops []state.PacketHop) hopGraph {
	var g hopGraph
	seen := map[string]bool{}
	add := func(from, to string, at uint32, ok bool) {
		if from == "" || to == "" || from == to {
			return
		}
		if len(g.Edges) >= maxGraphEdges {
			g.Dropped++
			return
		}
		g.Edges = append(g.Edges, hopEdge{From: from, To: to, AtMs: at, OK: ok})
		seen[from], seen[to] = true, true
	}
	for _, h := range hops {
		for _, n := range h.Heard {
			add(h.By, n, h.AtMs, true)
		}
		for _, n := range h.MissedBy {
			add(h.By, n, h.AtMs, false)
		}
		// A transmission nobody was in range of still happened, and the node
		// that made it belongs on the graph.
		if len(h.Heard) == 0 && len(h.MissedBy) == 0 && h.By != "" {
			seen[h.By] = true
		}
	}
	if len(seen) == 0 {
		return g
	}

	// Adjacency, for the walk.
	next := map[string][]string{}
	for _, e := range g.Edges {
		next[e.From] = append(next[e.From], e.To)
	}

	layer := map[string]int{}
	start := origin
	if !seen[start] {
		// The origin may not appear as a transmitter — a packet opened mid
		// flight, say. Start from whoever transmitted first instead of
		// refusing to draw anything.
		if len(hops) > 0 {
			start = hops[0].By
		}
	}
	if seen[start] {
		layer[start] = 0
		queue := []string{start}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, to := range next[cur] {
				if _, done := layer[to]; done {
					continue
				}
				layer[to] = layer[cur] + 1
				queue = append(queue, to)
			}
		}
	}
	// Anything the walk never reached — a node that only ever appears as a
	// receiver of a transmitter we never saw — goes one past the deepest
	// thing that does reach it, or at the end.
	deepest := 0
	for _, l := range layer {
		if l > deepest {
			deepest = l
		}
	}
	var orphans []string
	for n := range seen {
		if _, ok := layer[n]; !ok {
			orphans = append(orphans, n)
		}
	}
	sort.Strings(orphans)
	for _, n := range orphans {
		layer[n] = deepest + 1
	}

	// Rows within a layer, in first-seen order so the picture is stable
	// between frames and between runs.
	order := map[string]int{}
	var appear []string
	for _, h := range hops {
		for _, n := range append([]string{h.By}, append(h.Heard, h.MissedBy...)...) {
			if n == "" || order[n] != 0 || (len(appear) > 0 && appear[0] == n) {
				continue
			}
			order[n] = len(appear)
			appear = append(appear, n)
		}
	}
	rows := map[int]int{}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if layer[names[i]] != layer[names[j]] {
			return layer[names[i]] < layer[names[j]]
		}
		return order[names[i]] < order[names[j]]
	})
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

// counts is how it went, for the caption.
func (g hopGraph) counts() (ok, failed int) {
	for _, e := range g.Edges {
		if e.OK {
			ok++
		} else {
			failed++
		}
	}
	return ok, failed
}

// drawHopGraph paints the graph into the space it is given.
//
// Fixed height rather than flexed: this sits above the journey table, and a
// graph that grows until the table is off the bottom of a third-of-a-window
// panel is worse than a small graph.
func drawHopGraph(t *theme.Theme, gtx layout.Context, g hopGraph) layout.Dimensions {
	h := gtx.Dp(graphHeight)
	w := gtx.Constraints.Max.X
	sz := image.Pt(w, h)
	if len(g.Nodes) == 0 || w <= 0 {
		return layout.Dimensions{Size: image.Pt(w, 0)}
	}

	// Asymmetric on purpose. Labels are drawn to the right of their node, so
	// the last layer needs a label's width of room or its names run off the
	// panel — which is what the first version did.
	padL, padY := float32(gtx.Dp(14)), float32(gtx.Dp(18))
	padR := float32(gtx.Dp(labelWidth + 12))
	stepX := float32(0)
	if g.Layers > 1 {
		stepX = (float32(w) - padL - padR) / float32(g.Layers-1)
	}
	stepY := float32(0)
	if g.Wide > 1 {
		stepY = (float32(h) - 2*padY) / float32(g.Wide-1)
	}
	pos := func(n hopNode) f32.Point {
		x := padL + float32(n.Layer)*stepX
		// Centre each layer vertically: a layer of one node should sit on the
		// middle line, not at the top.
		rowsHere := 0
		for _, m := range g.Nodes {
			if m.Layer == n.Layer {
				rowsHere++
			}
		}
		off := (float32(g.Wide-rowsHere) / 2) * stepY
		y := padY + off + float32(n.Row)*stepY
		if g.Wide == 1 {
			y = float32(h) / 2
		}
		return f32.Pt(x, y)
	}

	// Edges first, so nodes sit on top of them.
	//
	// One path at a time: Gio allows only a single clip.Path open on an Ops,
	// and beginning a second before the first ends panics with "cannot mix
	// multi ops with single ones" — which is a crash on screen and nothing at
	// all in a unit test.
	var okPath clip.Path
	okPath.Begin(gtx.Ops)
	for _, e := range g.Edges {
		if !e.OK {
			continue
		}
		a, aok := g.at(e.From)
		b, bok := g.at(e.To)
		if !aok || !bok {
			continue
		}
		comp.Segment(&okPath, pos(a), pos(b), 1.6)
	}
	paint.FillShape(gtx.Ops, theme.Alpha(t.P.Accent, 0.75),
		clip.Outline{Path: okPath.End()}.Op())

	var badPath clip.Path
	badPath.Begin(gtx.Ops)
	for _, e := range g.Edges {
		if e.OK {
			continue
		}
		a, aok := g.at(e.From)
		b, bok := g.at(e.To)
		if !aok || !bok {
			continue
		}
		// Thin and faint rather than dashed. The first version drew five short
		// segments per edge, which is legible for one edge and tiles into a
		// solid block when twenty of them fan onto neighbouring points.
		comp.Segment(&badPath, pos(a), pos(b), 0.9)
	}
	paint.FillShape(gtx.Ops, theme.Alpha(t.P.Bad, 0.45),
		clip.Outline{Path: badPath.End()}.Op())

	// Nodes.
	var dots clip.Path
	dots.Begin(gtx.Ops)
	r := float32(gtx.Dp(4))
	for _, n := range g.Nodes {
		comp.Disc(&dots, pos(n), r)
	}
	paint.FillShape(gtx.Ops, t.P.Ink, clip.Outline{Path: dots.End()}.Op())

	// Labels, only where there is room for them. A layer packed with twenty
	// nodes gets dots and no names rather than a smear of overlapping text.
	if stepY >= float32(gtx.Dp(18)) || g.Wide == 1 {
		for _, n := range g.Nodes {
			p := pos(n)
			off := op.Offset(image.Pt(int(p.X)+gtx.Dp(7), int(p.Y)-gtx.Dp(6))).Push(gtx.Ops)
			gtx2 := gtx
			gtx2.Constraints.Max.X = gtx.Dp(labelWidth)
			comp.OneLine(t, t.Sz.Caption, t.P.Dim, shortName(n.Name), false)(gtx2)
			off.Pop()
		}
	}
	return layout.Dimensions{Size: sz}
}

// labelWidth is how much room a node's name gets, and therefore how much
// space the drawing reserves on its right.
const labelWidth = 104

// graphHeight is the space the picture gets. Enough to be read, small enough
// that the table under it stays on screen in a docked panel.
const graphHeight = 190

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
	if g.Hidden > 0 {
		s += fmt.Sprintf("  ·  %d nodes not drawn", g.Hidden)
	}
	if g.Dropped > 0 {
		s += fmt.Sprintf("  ·  %d more edges not drawn", g.Dropped)
	}
	return s
}
