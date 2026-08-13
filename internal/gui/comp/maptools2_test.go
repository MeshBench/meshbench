package comp

import (
	"testing"

	"gioui.org/f32"
	"gioui.org/io/pointer"
)

// What each tool does, and what it must not do.
//
// Nothing read the chosen tool at all: every mode dragged a node, which is how
// a repeater could be walked across the map while the toolbar said "link", and
// place and link did nothing whatever.

// nodeAt finds where a node was drawn, so a test aims at the node rather than
// at a coordinate written down in advance.
func (h *mapHarness) nodeAt(name string) f32.Point {
	pts := h.mv.project(h.sn, h.sz)
	for _, p := range pts {
		if p.n.Name == name {
			return f32.Pt(p.x, p.y)
		}
	}
	return f32.Pt(-1, -1)
}

func (h *mapHarness) drag(from, to f32.Point) {
	// The pointer has to arrive before it can be pressed: Gio tracks a drag
	// from the press that started it, and a drag with no press behind it is
	// not a gesture the router will accept.
	h.r.Queue(pointer.Event{Kind: pointer.Move, Position: from, Source: pointer.Mouse})
	h.frame()
	h.r.Queue(pointer.Event{Kind: pointer.Press, Position: from,
		Source: pointer.Mouse, Buttons: pointer.ButtonPrimary})
	h.frame()
	// A move while pressed: the router derives the drag itself, and refuses a
	// Drag handed to it directly.
	h.r.Queue(pointer.Event{Kind: pointer.Move, Position: to,
		Source: pointer.Mouse, Buttons: pointer.ButtonPrimary})
	h.frame()
	h.r.Queue(pointer.Event{Kind: pointer.Release, Position: to,
		Source: pointer.Mouse, Buttons: pointer.ButtonPrimary})
	h.frame()
}

// Only the move tool moves a node.
func TestOnlyTheMoveToolMovesANode(t *testing.T) {
	for _, tool := range []string{"select", "link", "place", "measure"} {
		h := newMapHarness()
		h.mv.Tool = tool
		moved := ""
		h.mv.OnMove = func(name string, lat, lon float64) { moved = name }
		h.frame()

		at := h.nodeAt("Abernethy Repeater")
		h.drag(at, at.Add(f32.Pt(60, 40)))
		if moved != "" {
			t.Errorf("the %s tool dragged %q; only move moves a node", tool, moved)
		}
	}

	h := newMapHarness()
	h.mv.Tool = "move"
	moved := ""
	h.mv.OnMove = func(name string, lat, lon float64) { moved = name }
	h.frame()
	at := h.nodeAt("Abernethy Repeater")
	h.drag(at, at.Add(f32.Pt(60, 40)))
	if moved != "Abernethy Repeater" {
		t.Errorf("the move tool moved %q, not the node under the pointer", moved)
	}
}

// The place tool puts something where it was clicked.
func TestThePlaceToolReportsWhereItWasClicked(t *testing.T) {
	h := newMapHarness()
	h.mv.Tool = "place"
	var gotLat, gotLon float64
	placed := false
	h.mv.OnPlace = func(lat, lon float64) { gotLat, gotLon, placed = lat, lon, true }
	h.frame()

	h.press(f32.Pt(400, 300), pointer.ButtonPrimary)
	if !placed {
		t.Fatal("clicking with the place tool reached nothing")
	}
	if gotLat == 0 || gotLon == 0 {
		t.Errorf("placed at %v,%v, which is not where the map was clicked", gotLat, gotLon)
	}
}

// The link tool takes two nodes and asks about the pair.
func TestTheLinkToolTakesTwoNodes(t *testing.T) {
	h := newMapHarness()
	h.mv.Tool = "link"
	var pair [2]string
	h.mv.OnLinkPair = func(a, b string) { pair = [2]string{a, b} }
	h.mv.OnSelect = func(names []string, additive bool) {}
	h.frame()

	h.press(h.nodeAt("Abernethy Repeater"), pointer.ButtonPrimary)
	if pair[0] != "" {
		t.Fatal("one node was enough to ask about a link, which takes two")
	}
	h.press(h.nodeAt("Bishop Hill"), pointer.ButtonPrimary)
	if pair[0] != "Abernethy Repeater" || pair[1] != "Bishop Hill" {
		t.Fatalf("the link tool reported %v after two nodes were clicked", pair)
	}
}
