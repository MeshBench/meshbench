package nodeview

import (
	"testing"

	"gioui.org/f32"

	"github.com/MeshBench/meshbench/internal/ui/uitest"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// Can you type into it?
//
// Reported: the filter in the node window does not accept typing. A screenshot
// cannot answer that - the box is drawn either way - so this drives the panel
// through Gio's own input router: lay out, click the box, send characters, lay
// out again, and read the editor back.

// The node view's filter has to accept characters.
func TestTypingIntoTheNodeFilter(t *testing.T) {
	p := &ViewPanel{}
	snap := &state.Snapshot{Stats: []state.NodeStat{
		{Name: "Abernethy Repeater", Backend: "native", Running: true, RSSBytes: 4 << 20},
		{Name: "Bishop Hill", Backend: "native", Running: true, RSSBytes: 4 << 20},
	}}
	h := uitest.New(p.Draw, snap)
	h.Frame()

	// The filter box sits under the row of checkboxes, near the top left.
	h.Click(f32.Pt(200, 46))
	h.TypeText("bishop")

	if got := p.tb.Filter.Text(); got != "bishop" {
		t.Fatalf("filter holds %q after typing \"bishop\"; the box does not accept input", got)
	}
}

// The Nodes list filter, which is the one that would not accept a keystroke.
//
// It built a comp.Field inside Draw and copied the editor in and out by value,
// so every frame produced a different widget at a different address. Gio keys
// focus on that address: the click focused an editor that no longer existed by
// the time the characters arrived. The box drew correctly throughout.
func TestTypingIntoTheNodesListFilter(t *testing.T) {
	np := &Panel{}
	snap := &state.Snapshot{Nodes: []state.Node{
		{Name: "Abernethy Repeater", Kind: "simple-repeater"},
		{Name: "Bishop Hill", Kind: "simple-repeater"},
		{Name: "AngusOutlaw1", Kind: "companion"},
	}}
	h := uitest.New(np.Draw, snap)
	h.Frame()

	h.Click(f32.Pt(150, 20))
	h.TypeText("bishop")

	got := np.tbl.Filter.Text()
	if got != "bishop" {
		t.Fatalf("filter holds %q after typing; the box does not accept input", got)
	}
	// And it has to actually filter, not merely hold text.
	h.Frame()
	if n := len(np.tbl.ShownKeys()); n != 1 {
		t.Errorf("filter %q leaves %d rows, want 1", got, n)
	}
}
