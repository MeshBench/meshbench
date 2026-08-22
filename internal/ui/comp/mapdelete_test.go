package comp

import (
	"testing"

	"gioui.org/io/key"
)

// Delete removes the selection, and it has to reach the map at all.
//
// The map's key filters name no focus target, which is what lets a text field
// keep the key while it is being typed into - and is also the thing that makes
// "does this arrive" worth a test rather than an assumption.

func TestDeleteKeyReportsTheSelection(t *testing.T) {
	h := newMapHarness()
	h.sn.Nodes[0].Selected = true
	var got []string
	h.mv.OnDelete = func(names []string) { got = names }
	h.frame()

	h.r.Queue(key.Event{Name: key.NameDeleteForward, State: key.Press})
	h.frame()
	if len(got) != 1 || got[0] != h.sn.Nodes[0].Name {
		t.Fatalf("Delete reported %v, want just %q", got, h.sn.Nodes[0].Name)
	}
}

// The Mac keyboard's delete key is Gio's DeleteBackward. A shortcut that works
// on one keyboard and not another reads as one that does not work.
func TestBackspaceDeletesToo(t *testing.T) {
	h := newMapHarness()
	h.sn.Nodes[1].Selected = true
	fired := false
	h.mv.OnDelete = func([]string) { fired = true }
	h.frame()

	h.r.Queue(key.Event{Name: key.NameDeleteBackward, State: key.Press})
	h.frame()
	if !fired {
		t.Fatal("Backspace over a selected node reached nothing")
	}
}

// Nothing selected is not a delete. Pressing the key over an empty map used to
// be the shape of "it deleted something and I do not know what".
func TestDeleteWithNothingSelectedDoesNothing(t *testing.T) {
	h := newMapHarness()
	fired := false
	h.mv.OnDelete = func([]string) { fired = true }
	h.frame()

	h.r.Queue(key.Event{Name: key.NameDeleteForward, State: key.Press})
	h.frame()
	if fired {
		t.Fatal("Delete with no selection asked to remove something")
	}
}

// A box drag selects several, so the key has to carry several - one question
// about five nodes, not five questions.
func TestDeleteCarriesEverySelectedNode(t *testing.T) {
	h := newMapHarness()
	for i := range h.sn.Nodes {
		h.sn.Nodes[i].Selected = true
	}
	var got []string
	h.mv.OnDelete = func(names []string) { got = names }
	h.frame()

	h.r.Queue(key.Event{Name: key.NameDeleteForward, State: key.Press})
	h.frame()
	if len(got) != len(h.sn.Nodes) {
		t.Fatalf("Delete reported %d of %d selected nodes: %v",
			len(got), len(h.sn.Nodes), got)
	}
}

// The right-click menu offers it as well, because a keyboard shortcut nobody
// has been told about is not a way of doing something.
func TestTheNodeMenuOffersDelete(t *testing.T) {
	for _, it := range menuFor("Abernethy Repeater") {
		if it.Action == "nodes.delete" {
			return
		}
	}
	t.Fatal("a right-click on a node offers no way to delete it")
}
