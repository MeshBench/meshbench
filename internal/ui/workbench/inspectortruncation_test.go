// The Inspector names the row under the pointer in full.
//
// Stayed in workbench when the node view moved out: this is about eventsPanel,
// which is the workbench's own.
package workbench

import (
	"image"
	"testing"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/uitest"
)

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
	h := uitest.New(p.Draw, snap)
	h.Size = image.Pt(340, 400)
	h.Frame()

	want := "sco-fif-aberdour-east -> sco-fif-aberdour-west"
	for y := float32(20); y < 380; y += 4 {
		h.Hover(f32.Pt(80, y))
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
