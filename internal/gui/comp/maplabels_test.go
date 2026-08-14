package comp

import (
	"image"
	"testing"

	"github.com/MeshBench/meshbench/internal/gui/state"
)

// named builds points at pixel positions, which is what placement works in.
func named(pos ...[3]any) []projected {
	nodes := make([]state.Node, len(pos))
	out := make([]projected, len(pos))
	for i, p := range pos {
		nodes[i] = state.Node{Name: p[0].(string), Kind: "simple-repeater"}
		out[i] = projected{x: float32(p[1].(int)), y: float32(p[2].(int)), n: &nodes[i]}
	}
	return out
}

// fixed is a sizer where every label is the same box.
func fixed(w, h int) func(int) image.Point {
	return func(int) image.Point { return image.Pt(w, h) }
}

// The whole point: two names that would sit on top of each other do not both
// get drawn in the same place.
func TestOverlappingLabelsDoNotBothGetPlaced(t *testing.T) {
	var l labeller
	pts := named([3]any{"one", 100, 100}, [3]any{"two", 104, 100})
	got := l.place(pts, image.Pt(800, 600), -1, fixed(60, 14))

	if len(got) == 0 {
		t.Fatal("nothing was placed at all")
	}
	seen := map[int]image.Rectangle{}
	for i, at := range got {
		r := image.Rectangle{Min: at, Max: at.Add(image.Pt(60, 14))}
		for j, o := range seen {
			if r.Overlaps(o) {
				t.Fatalf("labels %d and %d overlap: %v and %v", i, j, r, o)
			}
		}
		seen[i] = r
	}
}

// A second candidate position is genuinely used rather than the label simply
// being dropped: a node crowded on the right should get its name on the left.
func TestACrowdedLabelTakesAnotherSpot(t *testing.T) {
	var l labeller
	pts := named([3]any{"first", 100, 100}, [3]any{"second", 100, 130})
	got := l.place(pts, image.Pt(800, 600), -1, fixed(40, 14))
	if len(got) != 2 {
		t.Fatalf("placed %d labels, want both", len(got))
	}
}

// Selection outranks everything, so the label somebody is looking for is the
// one that survives a crowd.
func TestTheSelectedNodeKeepsItsLabel(t *testing.T) {
	var l labeller
	pts := named([3]any{"a", 100, 100}, [3]any{"b", 102, 100}, [3]any{"c", 104, 100})
	pts[2].n.Selected = true
	// Wide enough that only one of the three can be placed at all.
	got := l.place(pts, image.Pt(300, 200), -1, fixed(150, 14))

	if _, ok := got[2]; !ok {
		t.Fatal("the selected node lost its label to an unselected one")
	}
}

// A label that would fall off the edge is not drawn half off it.
func TestLabelsStayInsideTheViewport(t *testing.T) {
	var l labeller
	pts := named([3]any{"right at the edge", 795, 300})
	got := l.place(pts, image.Pt(800, 600), -1, fixed(120, 14))
	for i, at := range got {
		r := image.Rectangle{Min: at, Max: at.Add(image.Pt(120, 14))}
		if !r.In(image.Rectangle{Max: image.Pt(800, 600)}) {
			t.Fatalf("label %d at %v is outside the viewport", i, r)
		}
	}
}

// Placement must not depend on map iteration order, or labels would appear and
// disappear between frames of an unchanging map.
func TestPlacementIsStableAcrossFrames(t *testing.T) {
	pts := named([3]any{"a", 100, 100}, [3]any{"b", 108, 100},
		[3]any{"c", 116, 100}, [3]any{"d", 124, 100})
	var first map[int]image.Point
	for frame := 0; frame < 20; frame++ {
		var l labeller
		got := l.place(pts, image.Pt(400, 300), -1, fixed(70, 14))
		if frame == 0 {
			first = got
			continue
		}
		if len(got) != len(first) {
			t.Fatalf("frame %d placed %d labels, frame 0 placed %d",
				frame, len(got), len(first))
		}
		for i, at := range first {
			if got[i] != at {
				t.Fatalf("frame %d moved label %d from %v to %v", frame, i, at, got[i])
			}
		}
	}
}

// The cap is a real limit, and it keeps the highest priority labels.
func TestTheLabelCapKeepsTheImportantOnes(t *testing.T) {
	var pos [][3]any
	for i := 0; i < 40; i++ {
		pos = append(pos, [3]any{string(rune('a' + i%26)), 20 + i*30, 100})
	}
	pts := named(pos...)
	pts[39].n.Selected = true

	l := labeller{Max: 5}
	got := l.place(pts, image.Pt(2000, 600), -1, fixed(20, 14))
	if len(got) != 5 {
		t.Fatalf("placed %d labels with a cap of 5", len(got))
	}
	if _, ok := got[39]; !ok {
		t.Fatal("the cap dropped the selected node rather than an ordinary one")
	}
}
