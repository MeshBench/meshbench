package workbench

import (
	"fmt"
	"sort"

	"image/color"
	"strings"

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
	// Origin is the node that sent the shallowest-hop transmission in the
	// journey - the packet's true first hop, not necessarily whichever relay
	// the clicked frame instance happens to be. Drawn inside the graph at its
	// own layer rather than as a decoration off to the side, and marked out
	// by colour rather than position: a flood's origin is not always alone at
	// the left of the picture once several relays share a layer.
	Origin string
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
	hops := pk.Journey

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
	minHop := -1
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
		// The journey's true first hop, not whichever relay the clicked frame
		// instance happens to be: those are the same node only when the
		// packet clicked into this view is the original transmission itself.
		if minHop == -1 || h.Hops < minHop {
			minHop, g.Origin = h.Hops, h.By
		}
	}
	if len(layer) == 0 {
		return g
	}

	within := func(name string) bool {
		return maxHops <= 0 || layer[name] <= maxHops
	}
	// Uncapped here on purpose: the cap has to see the same edges collapsing
	// is about to reduce to one per receiver, not a raw per-attempt count a
	// busy flood can run into the hundreds - capping first could drop the one
	// edge that would have survived collapsing and leave a node's true first
	// acceptance invisible in favour of an earlier miss that happened to be
	// counted first.
	var raw []hopEdge
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
		raw = append(raw, hopEdge{From: from, To: to, AtMs: at, Hop: hop, Why: k})
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

	// One edge per receiver: whichever attempt answers "did this node ever
	// get the message" - its first success if it ever had one, or else its
	// first reason for missing it. A node offered the same flood on every
	// hop otherwise draws one edge per attempt, and a five-hop flood across a
	// real mesh turns into exactly the tangle of crossing, mostly-redundant
	// lines this picture exists to avoid. Not counted toward g.Dropped: a
	// collapsed duplicate is not lost information, since the reception ledger
	// still has the whole history for whoever wants it - only cap overflow
	// belongs in that count.
	g.Edges = collapseEdgesPerReceiver(raw)
	if len(g.Edges) > maxGraphEdges {
		g.Dropped += len(g.Edges) - maxGraphEdges
		g.Edges = g.Edges[:maxGraphEdges]
	}

	// A node's layer has to agree with the one edge actually drawn to it, or
	// the picture skips or backtracks a column - or a ring, in the radial
	// layout - for no reason a viewer can see. layer above is each node's
	// minimum hop across every attempt it was ever part of; collapsing can
	// keep a *different* attempt's edge than the one that set that minimum,
	// and the two disagreeing is exactly the bug this fixes: a node placed
	// two columns out with its only drawn edge arriving from the origin
	// directly. Walking the collapsed edges outward from the origin cannot
	// disagree with itself by construction. layer is left as a fallback for
	// whatever this walk does not reach - a transmitter the collapsed set
	// somehow has no path back to the origin through, which well-formed
	// flood data should not produce.
	refineLayersFromOrigin(g.Origin, g.Edges, layer)

	// Rows within a layer. First-seen order seeds it - deterministic, and a
	// sane fallback for a node with nothing to average against - and then
	// barycenterOrder pulls each layer toward the nodes it actually connects
	// to, which is what a hand-drawn diagram would do and "first seen"
	// never did. Left at first-seen order alone, two branches that never
	// interact still cross whenever the log happened to interleave them.
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

	// Which node a surviving edge actually touches. A node is placed in
	// layer above purely for being named in some hop's Heard or MissedBy -
	// before the edge cap and the outcome filters have had their say - so
	// without this check a node whose only edge got dropped by the cap, or
	// filtered out by an unchecked outcome, still draws as a dot with
	// nothing connecting to it: a node on the ring with no story.
	connected := map[string]bool{}
	for _, e := range g.Edges {
		connected[e.From] = true
		connected[e.To] = true
	}
	names := make([]string, 0, len(layer))
	for n := range layer {
		if !within(n) {
			continue
		}
		// The origin is the one node the picture always keeps even with
		// zero edges: a transmission nobody heard is still a transmission.
		if n != g.Origin && !connected[n] {
			g.Hidden++
			continue
		}
		names = append(names, n)
	}
	pos := barycenterOrder(names, layer, order, g.Edges)
	sort.Slice(names, func(i, j int) bool {
		if layer[names[i]] != layer[names[j]] {
			return layer[names[i]] < layer[names[j]]
		}
		return pos[names[i]] < pos[names[j]]
	})
	kept, surviving, hidden := drawableSet(names, layer, pos, g.Edges, g.Origin)
	g.Edges, g.Hidden = surviving, g.Hidden+hidden

	rows := map[int]int{}
	for _, n := range names {
		if !kept[n] {
			continue
		}
		l := layer[n]
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
