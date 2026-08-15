package workbench

import (
	"fmt"
	"testing"

	"github.com/MeshBench/meshbench/internal/gui/state"
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

// Why a hop failed is what an operator acts on, so it is read from the engine's
// own words rather than re-derived.
func TestAMissCarriesTheEnginesOwnReason(t *testing.T) {
	for _, c := range []struct {
		what string
		want missKind
	}{
		{"SNR -7.2 dB against -10.0 dB needed at SF8", missWeak},
		{"would have decoded at 3.1 dB, lost to a stronger interferer", missCollided},
		{"its own transmitter was keyed; LoRa is half duplex", missDeaf},
		{"something nobody has written yet", missOther},
	} {
		if got := classifyMiss(c.what); got != c.want {
			t.Errorf("classifyMiss(%q) = %v, want %v", c.what, got, c.want)
		}
	}
}

func TestFilteringAReasonKeepsTheTally(t *testing.T) {
	h := hopAt("orig", 0, 0, []string{"a"}, []string{"weak", "busy"})
	h.MissWhy = []string{
		"SNR -9 dB against -10 dB needed at SF8",
		"lost to a stronger interferer",
	}
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
