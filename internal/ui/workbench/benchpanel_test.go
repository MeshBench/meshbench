package workbench

import (
	"testing"

	"gioui.org/f32"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// The App view could not say which companion it was about.
//
// Its table was laid out with no select handler, so a row click reached
// nothing; and the view carries neither the map nor the node list, which were
// the only two things that ever selected anything. So companion.connect
// refused for want of a node, and the console beside it asked for a selection
// the view had no way to make.

func benchSnapshot() *state.Snapshot {
	return &state.Snapshot{
		Nodes: []state.Node{
			{Name: "Abernethy Repeater", Kind: "repeater"},
			{Name: "AngusOutlaw1", Kind: "companion"},
			{Name: "AngusOutlaw2", Kind: "companion"},
		},
		Endpoints: []state.Endpoint{
			{Node: "AngusOutlaw2", Kind: "tcp", Addr: "127.0.0.1:5301", Attached: true},
		},
	}
}

func TestClickingACompanionSelectsIt(t *testing.T) {
	var picked []string
	p := &benchPanel{OnSelect: func(n string) { picked = append(picked, n) }}
	snap := benchSnapshot()
	h := newPanelHarness(p.Draw, snap)
	h.frame()
	h.frame()

	// Down the table, until a row reports itself. Scanned rather than
	// computed: where a row sits is the table's business.
	for y := float32(20); y < float32(h.sz.Y)-40 && len(picked) == 0; y += 4 {
		h.click(f32.Pt(120, y))
	}
	if len(picked) == 0 {
		t.Fatal("no click anywhere in the companion table reached the selection")
	}
	if picked[0] != "AngusOutlaw1" && picked[0] != "AngusOutlaw2" {
		t.Fatalf("clicking a row selected %q, which is not a companion in the table", picked[0])
	}
}

// The actions are about a companion, so a repeater selected elsewhere leaves
// them alone rather than offering to serve a client to something with no
// companion protocol at all.
func TestTheBenchActionsOnlyFollowACompanion(t *testing.T) {
	p := &benchPanel{}
	snap := benchSnapshot()
	snap.Nodes[0].Selected = true // the repeater
	h := newPanelHarness(p.Draw, snap)
	h.frame()
	h.frame()
	if p.tb.Selected != "" {
		t.Fatalf("a selected repeater became the bench's subject: %q", p.tb.Selected)
	}

	snap.Nodes[0].Selected = false
	snap.Nodes[2].Selected = true // a companion
	h.frame()
	h.frame()
	if p.tb.Selected != "AngusOutlaw2" {
		t.Fatalf("the bench's subject is %q, want the selected companion", p.tb.Selected)
	}
}

// A served companion offers to drop its clients rather than to serve again,
// and says where it is - the address is the whole point of the panel.
func TestTheBenchSaysWhereAServedCompanionIs(t *testing.T) {
	var fired []string
	p := &benchPanel{OnAction: func(a, n string) { fired = append(fired, a+" "+n) }}
	snap := benchSnapshot()
	snap.Nodes[2].Selected = true // AngusOutlaw2, which is served
	h := newPanelHarness(p.Draw, snap)
	h.frame()
	h.frame()
	if got := p.row("AngusOutlaw2").serve.Label; got != "drop clients" {
		t.Fatalf("a served companion offers %q; serving one twice is not what the button means", got)
	}
	// And an unserved one offers to serve.
	snap.Nodes[2].Selected = false
	snap.Nodes[1].Selected = true // AngusOutlaw1, not served
	h.frame()
	h.frame()
	if got := p.row("AngusOutlaw1").serve.Label; got != "serve TCP" {
		t.Fatalf("an unserved companion offers %q", got)
	}
}
