package workbench

import (
	"fmt"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func hop(by string, at uint32, heard, missed []string) state.PacketHop {
	return state.PacketHop{By: by, AtMs: at, Heard: heard, MissedBy: missed}
}

// hopAt is the same with the packet's own hop count, which is what layers it.
func hopAt(by string, at uint32, n int, heard, missed []string) state.PacketHop {
	h := hop(by, at, heard, missed)
	h.Hops = n
	return h
}

// pkt wraps journey rows as a packet.
func pkt(origin string, hops []state.PacketHop) *state.Packet {
	return &state.Packet{Origin: origin, Journey: hops}
}

func all(origin string, hops ...state.PacketHop) hopGraph {
	return buildHopGraph(pkt(origin, hops), 0, nil)
}

func TestTheGraphLayersByDistanceFromTheOrigin(t *testing.T) {
	g := all("orig",
		hopAt("orig", 0, 0, []string{"a"}, nil),
		hopAt("a", 1000, 1, []string{"b"}, nil),
		hopAt("b", 2000, 2, []string{"c"}, nil),
	)
	want := map[string]int{"orig": 0, "a": 1, "b": 2, "c": 3}
	for name, layer := range want {
		n, ok := g.at(name)
		if !ok {
			t.Fatalf("%s missing from the graph", name)
		}
		if n.Layer != layer {
			t.Errorf("%s at layer %d, want %d", name, n.Layer, layer)
		}
	}
	if g.Layers != 4 {
		t.Errorf("Layers = %d, want 4", g.Layers)
	}
}

// A node that failed to hear a relay is still on the graph, and still one hop
// further out than whoever sent it. Leaving it off would make the picture a
// record of successes, which is the thing CoreScope already does.
func TestAFailedHopIsAnEdgeNotAnAbsence(t *testing.T) {
	g := all("orig", hop("orig", 0, []string{"heard"}, []string{"deaf"}))
	if _, ok := g.at("deaf"); !ok {
		t.Fatal("the node that failed to decode is not on the graph")
	}
	okc, failed := g.counts()
	if okc != 1 || failed != 1 {
		t.Fatalf("counts = %d ok, %d failed; want 1 and 1", okc, failed)
	}
	for _, e := range g.Edges {
		if e.To == "deaf" && e.OK() {
			t.Error("the failed edge is marked as successful")
		}
	}
}

// Meshes loop. A longest-path layering would recurse; this must not.
func TestALoopDoesNotHangTheLayout(t *testing.T) {
	done := make(chan hopGraph, 1)
	go func() {
		done <- all("a",
			hopAt("a", 0, 0, []string{"b"}, nil),
			hopAt("b", 1, 1, []string{"c"}, nil),
			hopAt("c", 2, 2, []string{"d"}, nil),
			hopAt("d", 3, 3, []string{"a"}, nil),
		)
	}()
	g := <-done
	if len(g.Nodes) != 4 {
		t.Fatalf("got %d nodes, want 4", len(g.Nodes))
	}
	// The origin keeps layer 0 even though something points back at it.
	if n, _ := g.at("a"); n.Layer != 0 {
		t.Errorf("origin moved to layer %d", n.Layer)
	}
}

// A flood across a country has hundreds of edges and no useful picture. It is
// capped — and says how many it dropped, because a truncated graph that looks
// complete is worse than a small one that admits it.
func TestALargeFloodIsCappedAndSaysSo(t *testing.T) {
	const fanout = maxGraphEdges + 180
	var heard []string
	for i := 0; i < fanout; i++ {
		heard = append(heard, fmt.Sprintf("n%03d", i))
	}
	g := buildHopGraph(pkt("orig", []state.PacketHop{hop("orig", 0, heard, nil)}), 0, nil)
	if len(g.Edges) > maxGraphEdges {
		t.Fatalf("drew %d edges, cap is %d", len(g.Edges), maxGraphEdges)
	}
	if g.Dropped == 0 {
		t.Fatal("dropped edges silently")
	}
	if want := fanout - maxGraphEdges; g.Dropped != want {
		t.Errorf("Dropped = %d, want %d", g.Dropped, want)
	}
	cap := graphCaption(g)
	if !contains(cap, "not drawn") {
		t.Errorf("caption does not mention the dropped edges: %q", cap)
	}
}

// A transmission nobody was in range of is still a transmission.
func TestATransmissionNobodyHeardKeepsItsSender(t *testing.T) {
	g := buildHopGraph(pkt("orig", []state.PacketHop{hop("orig", 0, nil, nil)}), 0, nil)
	if _, ok := g.at("orig"); !ok {
		t.Fatal("the sender vanished when nobody heard it")
	}
}

// A packet opened mid-flight has an origin that never transmitted in the rows
// we hold. Draw what there is rather than nothing.
func TestAnUnknownOriginFallsBackToTheFirstTransmitter(t *testing.T) {
	g := all("somebody-else", hop("a", 0, []string{"b"}, nil))
	if n, ok := g.at("a"); !ok || n.Layer != 0 {
		t.Fatalf("expected 'a' at layer 0, got %+v (present=%v)", n, ok)
	}
}

// The clicked packet is one specific frame instance, which can be any relay
// along a flood - pk.Origin names whoever sent *that* frame, not necessarily
// the message's true first hop. The graph's origin has to be the journey's
// shallowest transmission regardless of which frame was clicked, or a relay
// clicked mid-flood draws as a node disconnected from its own edges.
func TestOriginIsTheJourneysTrueFirstHopNotTheClickedFrame(t *testing.T) {
	hops := []state.PacketHop{
		hopAt("orig", 0, 0, []string{"a"}, nil),
		hopAt("a", 100, 1, []string{"b"}, nil),
		hopAt("b", 200, 2, []string{"c"}, nil),
	}
	// pk.Origin says "b" - who sent the specific frame that was clicked into
	// this view - not "orig", who actually started the message.
	g := buildHopGraph(pkt("b", hops), 0, nil)
	if g.Origin != "orig" {
		t.Fatalf("Origin = %q, want %q", g.Origin, "orig")
	}
	if n, ok := g.at("orig"); !ok || n.Layer != 0 {
		t.Fatalf("expected 'orig' at layer 0, got %+v (present=%v)", n, ok)
	}
}

func TestAnEmptyJourneyDrawsNothing(t *testing.T) {
	g := buildHopGraph(pkt("orig", nil), 0, nil)
	if len(g.Nodes) != 0 || len(g.Edges) != 0 {
		t.Fatalf("expected an empty graph, got %d nodes and %d edges",
			len(g.Nodes), len(g.Edges))
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// A crowded layer is summarised rather than drawn. Twenty-five neighbours
// hearing one advert is a fan, not a shape, and drawing all of it turned the
// panel into hatching the first time this ran.
func TestACrowdedLayerIsCappedAndSaysSo(t *testing.T) {
	var heard, missed []string
	for i := 0; i < 6; i++ {
		heard = append(heard, fmt.Sprintf("h%d", i))
	}
	for i := 0; i < 20; i++ {
		missed = append(missed, fmt.Sprintf("m%d", i))
	}
	g := buildHopGraph(pkt("orig", []state.PacketHop{hop("orig", 0, heard, missed)}), 0, nil)

	rows := map[int]int{}
	for _, n := range g.Nodes {
		rows[n.Layer]++
	}
	for layer, n := range rows {
		if n > maxRowsPerLayer {
			t.Errorf("layer %d drew %d nodes, cap is %d", layer, n, maxRowsPerLayer)
		}
	}
	if g.Hidden == 0 {
		t.Fatal("hid nodes without saying so")
	}
	if c := graphCaption(g); !contains(c, "nodes not drawn") {
		t.Errorf("caption does not mention the hidden nodes: %q", c)
	}
}

// Layering is the packet's own hop count, not a breadth-first distance. A node
// that hears the origin directly and again at hop three is at both depths, and
// the flood really did reach hop three — the first version collapsed exactly
// this case into two layers, which is the bug that prompted the rewrite.
func TestDepthComesFromTheFramesHopCountNotTheGraph(t *testing.T) {
	g := all("orig",
		hopAt("orig", 0, 0, []string{"a", "far"}, nil),
		hopAt("a", 100, 1, []string{"b"}, nil),
		hopAt("b", 200, 2, []string{"c"}, nil),
		hopAt("c", 300, 3, []string{"far"}, nil),
	)
	if g.Deepest != 3 {
		t.Fatalf("Deepest = %d, want 3", g.Deepest)
	}
	if g.Layers < 4 {
		t.Errorf("Layers = %d, want at least 4", g.Layers)
	}
	// "far" heard the origin directly, so it stays at hop 1 however late it is
	// heard again.
	if n, _ := g.at("far"); n.Layer != 1 {
		t.Errorf("far at layer %d, want 1", n.Layer)
	}
}

// A row order taken straight from "first seen in the log" crosses whenever
// the log happens to interleave two branches that never interact - the bug
// the barycenter sweep replaced. Here the log sees b's branch (y) before
// a's (x), so first-seen order alone would put x's row after y's even
// though x's only parent is a and y's only parent is b - a crossing that
// has nothing to do with the mesh and everything to do with log order.
func TestRowOrderTracksParentsNotLogOrder(t *testing.T) {
	g := all("orig",
		hopAt("orig", 0, 0, []string{"a", "b"}, nil),
		hopAt("b", 100, 1, []string{"y"}, nil),
		hopAt("a", 101, 1, []string{"x"}, nil),
	)
	a, _ := g.at("a")
	b, _ := g.at("b")
	x, _ := g.at("x")
	y, _ := g.at("y")
	if (x.Row < y.Row) != (a.Row < b.Row) {
		t.Errorf("x/y did not settle to their parents' order: a.Row=%d b.Row=%d x.Row=%d y.Row=%d",
			a.Row, b.Row, x.Row, y.Row)
	}
}

// The hop limit is a view, not a redefinition: what it excludes is counted and
// reported rather than quietly vanishing.
func TestAHopLimitSaysWhatItLeftOut(t *testing.T) {
	hops := []state.PacketHop{
		hopAt("orig", 0, 0, []string{"a"}, nil),
		hopAt("a", 100, 1, []string{"b"}, nil),
		hopAt("b", 200, 2, []string{"c"}, nil),
	}
	full := buildHopGraph(pkt("orig", hops), 0, nil)
	cut := buildHopGraph(pkt("orig", hops), 1, nil)
	if len(cut.Nodes) >= len(full.Nodes) {
		t.Fatalf("limit drew %d nodes, unlimited drew %d", len(cut.Nodes), len(full.Nodes))
	}
	if cut.Beyond == 0 {
		t.Error("excluded hops without saying so")
	}
	if !contains(graphCaption(cut), "beyond the hop limit") {
		t.Errorf("caption is silent about the limit: %q", graphCaption(cut))
	}
	// The packet's true depth is still reported, so the caption cannot imply
	// the flood stopped where the drawing does.
	if cut.Deepest != full.Deepest {
		t.Errorf("Deepest changed with the view: %d vs %d", cut.Deepest, full.Deepest)
	}
}

// Why a hop failed is what an operator acts on, so it is drawn as the cause
// the engine established rather than re-derived from its wording.
func TestAMissCarriesTheEnginesOwnReason(t *testing.T) {
	for _, c := range []struct {
		class string
		want  missKind
	}{
		{"floor", missWeak},
		{"interference", missCollided},
		{"collision", missCollided},
		{"receiver-busy", missBusy},
		{"half-duplex", missDeaf},
		{"unclassified", missOther},
		{"", missOther},
	} {
		if got := missKindOf(c.class); got != c.want {
			t.Errorf("missKindOf(%q) = %v, want %v", c.class, got, c.want)
		}
	}
}

// A cause the engine did name is never drawn as a weak link: the picture and
// the events panel have to agree about what an operator should go and fix.
func TestACollisionIsNotDrawnAsAWeakSignal(t *testing.T) {
	for _, class := range []string{"collision", "receiver-busy", "unclassified"} {
		if got := missKindOf(class); got == missWeak {
			t.Errorf("missKindOf(%q) drew a %q miss as too weak", class, class)
		}
	}
}

func TestFilteringAReasonKeepsTheTally(t *testing.T) {
	h := hopAt("orig", 0, 0, []string{"a"}, []string{"weak", "busy"})
	h.MissWhy = []string{
		"SNR -9 dB against -10 dB needed at SF8",
		"lost to a stronger interferer",
	}
	h.MissClass = []string{"floor", "interference"}
	show := map[missKind]bool{missNone: true, missWeak: false, missCollided: true, missOther: true, missDeaf: true}
	g := buildHopGraph(pkt("orig", []state.PacketHop{h}), 0, show)

	for _, e := range g.Edges {
		if e.Why == missWeak {
			t.Error("drew a class that was filtered out")
		}
	}
	// Hiding a class from the picture must not change what the caption says
	// happened.
	if _, failed := g.counts(); failed != 2 {
		t.Errorf("failed count = %d, want 2 — the tally must survive the filter", failed)
	}
	if g.Total[missWeak] != 1 {
		t.Errorf("Total[missWeak] = %d, want 1", g.Total[missWeak])
	}
}

// The reason travels on the journey row, because a journey spans every
// transmission of the message while Fates covers one packet of it. Joining
// through Fates silently classified almost every failed hop as "other".
func TestEveryFailedHopOfALongFloodKeepsItsReason(t *testing.T) {
	var hops []state.PacketHop
	for i := 0; i < 6; i++ {
		h := hopAt(fmt.Sprintf("r%d", i), uint32(i*1000), i,
			[]string{fmt.Sprintf("r%d", i+1)}, []string{fmt.Sprintf("m%d", i)})
		h.MissWhy = []string{"SNR -9 dB against -10 dB needed at SF8"}
		h.MissClass = []string{"floor"}
		hops = append(hops, h)
	}
	g := buildHopGraph(pkt("r0", hops), 0, nil)
	if g.Total[missOther] != 0 {
		t.Errorf("%d hops fell through to \"other\"", g.Total[missOther])
	}
	if g.Total[missWeak] != 6 {
		t.Errorf("Total[missWeak] = %d, want 6", g.Total[missWeak])
	}
}

// A node offered the same message on more than one hop draws only one edge:
// whichever attempt answers "did this node ever get the message" - its first
// success if it had one, never an earlier miss standing in for it. This is
// the bug that prompted the collapse: a node that missed an early hop and
// was accepted on a later one must not end up looking like it never got the
// message just because a miss was recorded first.
func TestANodeAcceptedOnAnyHopDrawsItsAcceptedEdgeNotAnEarlierMiss(t *testing.T) {
	missed := hopAt("relay1", 100, 1, nil, []string{"target"})
	missed.MissWhy = []string{"its own transmitter was keyed; LoRa is half duplex"}
	missed.MissClass = []string{"half-duplex"}
	heard := hopAt("relay2", 200, 2, []string{"target"}, nil)
	g := all("orig",
		hopAt("orig", 0, 0, []string{"relay1", "relay2"}, nil),
		missed,
		heard,
	)
	var toTarget []hopEdge
	for _, e := range g.Edges {
		if e.To == "target" {
			toTarget = append(toTarget, e)
		}
	}
	if len(toTarget) != 1 {
		t.Fatalf("target has %d edges, want exactly 1: %+v", len(toTarget), toTarget)
	}
	if !toTarget[0].OK() {
		t.Errorf("target's one edge is a miss (%v), want the accepted one from relay2", toTarget[0].Why)
	}
}

// The columns view's own bug report: a node was placed at its shallowest
// ever-seen hop across every attempt, but collapsing can keep a *different*
// attempt's edge than the one that set that minimum - here, a miss straight
// from the origin sets the shallow placement, while the node's only surviving
// edge is a much later accept relayed through two hops. The node's layer has
// to follow the edge that is actually drawn, not the shallowest mention that
// lost out to it, or the picture draws a line that skips or runs backward
// across columns for no reason a viewer can see.
func TestANodesLayerAgreesWithItsOneDrawnEdge(t *testing.T) {
	origTx := hopAt("orig", 0, 0, []string{"relay1"}, []string{"target"})
	origTx.MissWhy = []string{"its own transmitter was keyed; LoRa is half duplex"}
	origTx.MissClass = []string{"half-duplex"}
	relay1Tx := hopAt("relay1", 100, 1, []string{"relay2"}, nil)
	relay2Tx := hopAt("relay2", 200, 2, []string{"target"}, nil)
	g := all("orig", origTx, relay1Tx, relay2Tx)

	n, ok := g.at("target")
	if !ok {
		t.Fatal("target is not on the graph")
	}
	if n.Layer != 3 {
		t.Errorf("target at layer %d, want 3 - one hop past relay2 (layer 2), matching its drawn edge", n.Layer)
	}
	for _, e := range g.Edges {
		if e.To == "target" && e.From != "relay2" {
			t.Errorf("target's edge is from %q, want relay2 - who it actually accepted the message from", e.From)
		}
	}
}

// The opposite case: a node that was never accepted still gets exactly one
// edge, carrying a reason rather than silently vanishing.
func TestANodeNeverAcceptedDrawsOneEdgeWithAReason(t *testing.T) {
	first := hopAt("relay1", 100, 1, nil, []string{"target"})
	first.MissWhy = []string{"its own transmitter was keyed; LoRa is half duplex"}
	first.MissClass = []string{"half-duplex"}
	second := hopAt("relay2", 200, 2, nil, []string{"target"})
	second.MissWhy = []string{"would have decoded at 3.1 dB, lost to a stronger interferer"}
	second.MissClass = []string{"interference"}
	g := all("orig",
		hopAt("orig", 0, 0, []string{"relay1", "relay2"}, nil),
		first,
		second,
	)
	var toTarget []hopEdge
	for _, e := range g.Edges {
		if e.To == "target" {
			toTarget = append(toTarget, e)
		}
	}
	if len(toTarget) != 1 {
		t.Fatalf("target has %d edges, want exactly 1: %+v", len(toTarget), toTarget)
	}
	if toTarget[0].OK() || toTarget[0].Why != missDeaf {
		t.Errorf("target's one edge is %v, want the first miss (deaf, from relay1)", toTarget[0].Why)
	}
}

// A layer wider than maxRowsPerLayer has to lose nodes, and *which* ones it
// loses decides whether the rest of the picture still parses. Drop a node
// that relayed onward and everything it fed turns up in the next column with
// nothing arriving at it - the parentless column this was reported as. The
// invariant that catches it: every drawn edge has two drawn endpoints, and
// every drawn node except the origin sits on at least one drawn edge.
func TestACappedLayerNeverOrphansTheNextColumn(t *testing.T) {
	var heard []string
	for i := 0; i < 20; i++ {
		heard = append(heard, fmt.Sprintf("n%02d", i))
	}
	hops := []state.PacketHop{hopAt("orig", 0, 0, heard, nil)}
	// A relay late in first-seen order, so a naive "first N rows" cap drops it.
	hops = append(hops, hopAt("n19", 100, 1, []string{"far"}, nil))
	g := buildHopGraph(pkt("orig", hops), 0, nil)

	drawn := map[string]bool{}
	for _, n := range g.Nodes {
		drawn[n.Name] = true
	}
	touched := map[string]bool{}
	for _, e := range g.Edges {
		if !drawn[e.From] || !drawn[e.To] {
			t.Errorf("edge %s->%s has an endpoint that is not drawn", e.From, e.To)
		}
		touched[e.From], touched[e.To] = true, true
	}
	for _, n := range g.Nodes {
		if n.Name != g.Origin && !touched[n.Name] {
			t.Errorf("%s is drawn with no edge at all - a node floating in a column", n.Name)
		}
	}
}
