package workbench

import (
	"fmt"
	"testing"

	"github.com/MeshBench/meshbench/internal/gui/state"
)

func hop(by string, at uint32, heard, missed []string) state.PacketHop {
	return state.PacketHop{By: by, AtMs: at, Heard: heard, MissedBy: missed}
}

func TestTheGraphLayersByDistanceFromTheOrigin(t *testing.T) {
	g := buildHopGraph("orig", []state.PacketHop{
		hop("orig", 0, []string{"a"}, nil),
		hop("a", 1000, []string{"b"}, nil),
		hop("b", 2000, []string{"c"}, nil),
	})
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
	g := buildHopGraph("orig", []state.PacketHop{
		hop("orig", 0, []string{"heard"}, []string{"deaf"}),
	})
	if _, ok := g.at("deaf"); !ok {
		t.Fatal("the node that failed to decode is not on the graph")
	}
	okc, failed := g.counts()
	if okc != 1 || failed != 1 {
		t.Fatalf("counts = %d ok, %d failed; want 1 and 1", okc, failed)
	}
	for _, e := range g.Edges {
		if e.To == "deaf" && e.OK {
			t.Error("the failed edge is marked as successful")
		}
	}
}

// Meshes loop. A longest-path layering would recurse; this must not.
func TestALoopDoesNotHangTheLayout(t *testing.T) {
	done := make(chan hopGraph, 1)
	go func() {
		done <- buildHopGraph("a", []state.PacketHop{
			hop("a", 0, []string{"b"}, nil),
			hop("b", 1, []string{"c"}, nil),
			hop("c", 2, []string{"d"}, nil),
			hop("d", 3, []string{"a"}, nil),
		})
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
	var heard []string
	for i := 0; i < 400; i++ {
		heard = append(heard, fmt.Sprintf("n%03d", i))
	}
	g := buildHopGraph("orig", []state.PacketHop{hop("orig", 0, heard, nil)})
	if len(g.Edges) > maxGraphEdges {
		t.Fatalf("drew %d edges, cap is %d", len(g.Edges), maxGraphEdges)
	}
	if g.Dropped == 0 {
		t.Fatal("dropped edges silently")
	}
	if want := 400 - maxGraphEdges; g.Dropped != want {
		t.Errorf("Dropped = %d, want %d", g.Dropped, want)
	}
	cap := graphCaption(g)
	if !contains(cap, "not drawn") {
		t.Errorf("caption does not mention the dropped edges: %q", cap)
	}
}

// A transmission nobody was in range of is still a transmission.
func TestATransmissionNobodyHeardKeepsItsSender(t *testing.T) {
	g := buildHopGraph("orig", []state.PacketHop{hop("orig", 0, nil, nil)})
	if _, ok := g.at("orig"); !ok {
		t.Fatal("the sender vanished when nobody heard it")
	}
}

// A packet opened mid-flight has an origin that never transmitted in the rows
// we hold. Draw what there is rather than nothing.
func TestAnUnknownOriginFallsBackToTheFirstTransmitter(t *testing.T) {
	g := buildHopGraph("somebody-else", []state.PacketHop{
		hop("a", 0, []string{"b"}, nil),
	})
	if n, ok := g.at("a"); !ok || n.Layer != 0 {
		t.Fatalf("expected 'a' at layer 0, got %+v (present=%v)", n, ok)
	}
}

func TestAnEmptyJourneyDrawsNothing(t *testing.T) {
	g := buildHopGraph("orig", nil)
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
	g := buildHopGraph("orig", []state.PacketHop{hop("orig", 0, heard, missed)})

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
