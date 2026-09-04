package nodeview

import (
	"image"
	"testing"

	"gioui.org/f32"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/uitest"
)

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
	np := &Panel{}
	h := uitest.New(np.Draw, &state.Snapshot{Nodes: collidingNodes})
	h.Size = image.Pt(340, 400)
	h.Frame()
	if np.said != "" {
		t.Fatalf("with nothing hovered the footer names %q", np.said)
	}

	// Down the rows, one at a time: whichever it lands on, the footer has to
	// be saying that row's whole name.
	found := map[string]bool{}
	for y := float32(60); y < 260; y += 6 {
		h.Hover(f32.Pt(60, y))
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
	np := &Panel{}
	h := uitest.New(np.Draw, &state.Snapshot{Nodes: collidingNodes})
	h.Size = image.Pt(340, 400)
	np.tbl.Selected = "sco-fif-aberdour-west"
	h.Frame()
	h.Frame()
	if np.said != "sco-fif-aberdour-west" {
		t.Errorf("the footer names %q with that row selected", np.said)
	}
}
