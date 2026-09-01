package workbench

import (
	"image"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// hover moves the pointer over a point and lets the frame settle.
//
// Twice, deliberately: the panels put their footer above the table in the flex
// so the footer is measured first, and the row under the pointer is therefore
// named on the frame after the one that found it.
func (h *panelHarness) hover(at f32.Point) {
	for i := 0; i < 2; i++ {
		h.r.Queue(pointer.Event{Kind: pointer.Move, Position: at})
		h.frame()
	}
}

// Two Fife nodes whose names are the same string once a rail has cut them.
var collidingNodes = []state.Node{
	{Name: "sco-fif-aberdour-east", Kind: "simple-repeater"},
	{Name: "sco-fif-aberdour-west", Kind: "simple-repeater"},
	{Name: "Abernethy Repeater", Kind: "simple-repeater"},
}

// The Nodes table cuts a name to its column and said the rest nowhere.
//
// Three of the shipped fixture's names are identical once cut, so the rows for
// two different repeaters read the same. The panel now reads the row under the
// pointer out in full underneath.
func TestTheNodesTableSaysTheWholeNameOfTheRowUnderThePointer(t *testing.T) {
	np := &nodesPanel{}
	h := newPanelHarness(np.Draw, &state.Snapshot{Nodes: collidingNodes})
	h.sz = image.Pt(340, 400)
	h.frame()
	if np.said != "" {
		t.Fatalf("with nothing hovered the footer names %q", np.said)
	}

	// Down the rows, one at a time: whichever it lands on, the footer has to
	// be saying that row's whole name.
	found := map[string]bool{}
	for y := float32(60); y < 260; y += 6 {
		h.hover(f32.Pt(60, y))
		if np.said != "" {
			found[np.said] = true
		}
	}
	for _, n := range collidingNodes {
		if !found[n.Name] {
			t.Errorf("pointing down the table never named %q; it named %v",
				n.Name, found)
		}
	}
}

// The selection holds the line when the pointer leaves, so an answer read once
// is still there a second later.
func TestTheNodesTableKeepsNamingTheSelectedRow(t *testing.T) {
	np := &nodesPanel{}
	h := newPanelHarness(np.Draw, &state.Snapshot{Nodes: collidingNodes})
	h.sz = image.Pt(340, 400)
	np.tbl.Selected = "sco-fif-aberdour-west"
	h.frame()
	h.frame()
	if np.said != "sco-fif-aberdour-west" {
		t.Errorf("the footer names %q with that row selected", np.said)
	}
}

// The Inspector's From and To are the two names a row is about, and in a rail
// they are cut to the same characters for half the fixture. The panel now
// reads the row under the pointer out in full at its foot.
func TestTheInspectorSaysTheWholePairOfTheRowUnderThePointer(t *testing.T) {
	p := &eventsPanel{compact: true, forNode: true}
	snap := &state.Snapshot{
		Nodes: []state.Node{
			{Name: "sco-fif-aberdour-east", Kind: "simple-repeater", Selected: true},
			{Name: "sco-fif-aberdour-west", Kind: "simple-repeater"},
		},
		Events: []state.Event{{
			From: "sco-fif-aberdour-east", To: "sco-fif-aberdour-west",
			Kind: "rx", Class: "received", AtMs: 100,
		}},
		EventTotal: 1,
	}
	h := newPanelHarness(p.Draw, snap)
	h.sz = image.Pt(340, 400)
	h.frame()

	want := "sco-fif-aberdour-east -> sco-fif-aberdour-west"
	for y := float32(20); y < 380; y += 4 {
		h.hover(f32.Pt(80, y))
		if p.pointedAt == want {
			return
		}
	}
	t.Errorf("pointing down the Inspector never said %q; it last said %q",
		want, p.pointedAt)
}

// The Inspector's From and To were a fixed 96dp wherever the panel was, so
// popping it out into a window bought no more of a name than the rail gave.
func TestTheInspectorNameColumnsUseTheWidthItHas(t *testing.T) {
	p := &eventsPanel{compact: true, forNode: true}
	var ops op.Ops
	narrow := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(340, 400))}
	wide := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(900, 400))}
	_, small, _, _ := p.colWidths(narrow)
	_, big, _, _ := p.colWidths(wide)
	if big <= small {
		t.Errorf("a 900px panel gives its names %d px, a 340px one %d", big, small)
	}
	if small < 96 {
		t.Errorf("a rail gives its names %d px, under the 96 floor", small)
	}
	// The two names and the two numbers still fit the panel, or the columns
	// after them slide off the edge.
	tw, fw, snr, pill := p.colWidths(wide)
	if got := tw + 2*fw + snr + pill; got > 900 {
		t.Errorf("the columns want %d px of a 900 px panel", got)
	}
}
