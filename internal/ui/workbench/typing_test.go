package workbench

import (
	"image"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/ui/theme/brandfont"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// Can you type into it?
//
// Reported: the filter in the node window does not accept typing. A screenshot
// cannot answer that - the box is drawn either way - so this drives the panel
// through Gio's own input router: lay out, click the box, send characters, lay
// out again, and read the editor back.

type panelHarness struct {
	draw func(*theme.Theme, layout.Context, *state.Snapshot) layout.Dimensions
	th   *theme.Theme
	r    input.Router
	ops  op.Ops
	sz   image.Point
	snap *state.Snapshot
}

func newPanelHarness(draw func(*theme.Theme, layout.Context, *state.Snapshot) layout.Dimensions,
	snap *state.Snapshot) *panelHarness {
	return &panelHarness{
		draw: draw,
		th: theme.New(theme.Dark, theme.Default,
			text.NewShaper(text.WithCollection(brandfont.Collection()))),
		sz:   image.Pt(1200, 800),
		snap: snap,
	}
}

func (h *panelHarness) frame() {
	h.ops.Reset()
	gtx := layout.Context{
		Ops:         &h.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(h.sz),
		Source:      h.r.Source(),
	}
	h.draw(h.th, gtx, h.snap)
	h.r.Frame(&h.ops)
}

func (h *panelHarness) click(at f32.Point) {
	h.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: at, Buttons: pointer.ButtonPrimary},
		pointer.Event{Kind: pointer.Release, Position: at, Buttons: pointer.ButtonPrimary},
	)
	h.frame()
}

func (h *panelHarness) typeText(s string) {
	h.r.Queue(key.EditEvent{Text: s})
	h.frame()
}

// The node view's filter has to accept characters.
func TestTypingIntoTheNodeFilter(t *testing.T) {
	p := &nodeViewPanel{}
	snap := &state.Snapshot{Stats: []state.NodeStat{
		{Name: "Abernethy Repeater", Backend: "native", Running: true, RSSBytes: 4 << 20},
		{Name: "Bishop Hill", Backend: "native", Running: true, RSSBytes: 4 << 20},
	}}
	h := newPanelHarness(p.Draw, snap)
	h.frame()

	// The filter box sits under the row of checkboxes, near the top left.
	h.click(f32.Pt(200, 46))
	h.typeText("bishop")

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
	np := &nodesPanel{}
	snap := &state.Snapshot{Nodes: []state.Node{
		{Name: "Abernethy Repeater", Kind: "simple-repeater"},
		{Name: "Bishop Hill", Kind: "simple-repeater"},
		{Name: "AngusOutlaw1", Kind: "companion"},
	}}
	h := newPanelHarness(np.Draw, snap)
	h.frame()

	h.click(f32.Pt(150, 20))
	h.typeText("bishop")

	got := np.tbl.Filter.Text()
	if got != "bishop" {
		t.Fatalf("filter holds %q after typing; the box does not accept input", got)
	}
	// And it has to actually filter, not merely hold text.
	h.frame()
	if n := len(np.tbl.ShownKeys()); n != 1 {
		t.Errorf("filter %q leaves %d rows, want 1", got, n)
	}
}
