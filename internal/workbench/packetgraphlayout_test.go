package workbench

import (
	"math"
	"testing"

	"gioui.org/layout"
	"gioui.org/unit"
)

// testGtx is enough of a layout.Context for the layout functions: they only
// ever call gtx.Dp, which reads Metric and touches nothing else.
func testGtx() layout.Context {
	return layout.Context{Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
}

func dist(a, b [2]float32) float32 {
	dx, dy := a[0]-b[0], a[1]-b[1]
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}

// TestLayoutRadialPlacesTheOriginAtTheCentre is the one thing every radial
// picture has to get right: the node every journey shares sits where the
// eye looks first.
func TestLayoutRadialPlacesTheOriginAtTheCentre(t *testing.T) {
	g := all("orig",
		hopAt("orig", 0, 0, []string{"a", "b"}, nil),
		hopAt("a", 100, 1, []string{"c"}, nil),
	)
	pos := layoutRadial(testGtx(), g, 400, 300)
	p, ok := pos["orig"]
	if !ok {
		t.Fatal("origin has no position")
	}
	if p.X != 200 || p.Y != 150 {
		t.Errorf("origin at (%.1f, %.1f), want the centre (200, 150)", p.X, p.Y)
	}
}

// TestLayoutRadialRingsByHop is the layout's whole premise: distance from
// the origin has to be hop depth and nothing else.
func TestLayoutRadialRingsByHop(t *testing.T) {
	g := all("orig",
		hopAt("orig", 0, 0, []string{"a", "b"}, nil),
		hopAt("a", 100, 1, []string{"c"}, nil),
	)
	pos := layoutRadial(testGtx(), g, 400, 300)
	centre := pos["orig"]
	da := dist([2]float32{pos["a"].X, pos["a"].Y}, [2]float32{centre.X, centre.Y})
	db := dist([2]float32{pos["b"].X, pos["b"].Y}, [2]float32{centre.X, centre.Y})
	dc := dist([2]float32{pos["c"].X, pos["c"].Y}, [2]float32{centre.X, centre.Y})
	if math.Abs(float64(da-db)) > 0.01 {
		t.Errorf("two hop-1 nodes at different radii: a=%.2f b=%.2f", da, db)
	}
	if dc <= da {
		t.Errorf("hop-2 node c (radius %.2f) is not further out than hop-1 (radius %.2f)", dc, da)
	}
}

// TestLayoutRadialGroupsAChildNearItsParent is the point of the barycenter
// angle: a and its child c should land close together in angle, not
// scattered to opposite sides of the ring, or the rings read as noise
// rather than as branches.
func TestLayoutRadialGroupsAChildNearItsParent(t *testing.T) {
	g := all("orig",
		hopAt("orig", 0, 0, []string{"a", "b", "d", "e"}, nil),
		hopAt("a", 100, 1, []string{"c"}, nil),
	)
	pos := layoutRadial(testGtx(), g, 400, 300)
	centre := pos["orig"]
	angleOf := func(name string) float64 {
		p := pos[name]
		return math.Atan2(float64(p.Y-centre.Y), float64(p.X-centre.X))
	}
	angDiff := func(x, y float64) float64 {
		d := math.Abs(x - y)
		if d > math.Pi {
			d = 2*math.Pi - d
		}
		return d
	}
	toA := angDiff(angleOf("a"), angleOf("c"))
	toB := angDiff(angleOf("b"), angleOf("c"))
	if toA >= toB {
		t.Errorf("c (angle from a: %.2f rad) is not closer to its parent a than to unrelated sibling b (%.2f rad)", toA, toB)
	}
}

// TestLayoutForcePositionsEveryNodeFinitely is the baseline correctness bar
// for a physics layout: nothing escapes to infinity or comes out NaN, and
// every node the graph names gets an answer.
func TestLayoutForcePositionsEveryNodeFinitely(t *testing.T) {
	g := all("orig",
		hopAt("orig", 0, 0, []string{"a", "b"}, nil),
		hopAt("a", 100, 1, []string{"c"}, nil),
		hopAt("b", 100, 1, []string{"c"}, nil),
	)
	pos, _ := seedForce(g, 400, 300)
	if len(pos) != len(g.Nodes) {
		t.Fatalf("got %d positions, want one per node (%d)", len(pos), len(g.Nodes))
	}
	for _, n := range g.Nodes {
		p, ok := pos[n.Name]
		if !ok {
			t.Errorf("%s has no position", n.Name)
			continue
		}
		if math.IsNaN(float64(p.X)) || math.IsNaN(float64(p.Y)) ||
			math.IsInf(float64(p.X), 0) || math.IsInf(float64(p.Y), 0) {
			t.Errorf("%s settled to a non-finite position: %+v", n.Name, p)
		}
	}
}

// TestLayoutForceIsDeterministic matters because the same journey opened
// twice - or redrawn on the next tick while it keeps propagating - has to
// settle to the same picture. A random seed would make the graph visibly
// jump on every rebuild.
func TestLayoutForceIsDeterministic(t *testing.T) {
	g := all("orig",
		hopAt("orig", 0, 0, []string{"a", "b", "c"}, nil),
		hopAt("a", 100, 1, []string{"d"}, nil),
	)
	p1, _ := seedForce(g, 400, 300)
	p2, _ := seedForce(g, 400, 300)
	for name, a := range p1 {
		b, ok := p2[name]
		if !ok || a != b {
			t.Errorf("%s settled differently between two runs: %+v vs %+v (present=%v)", name, a, b, ok)
		}
	}
}

// TestLayoutForceSeparatesUnconnectedNodes: repulsion alone, with no edge
// pulling two nodes together, should keep them apart rather than collapsing
// them onto the same point.
func TestLayoutForceSeparatesUnconnectedNodes(t *testing.T) {
	g := all("orig", hopAt("orig", 0, 0, []string{"a", "b", "c", "d", "e"}, nil))
	pos, _ := seedForce(g, 400, 300)
	for i, ni := range g.Nodes {
		for j, nj := range g.Nodes {
			if i >= j {
				continue
			}
			d := dist([2]float32{pos[ni.Name].X, pos[ni.Name].Y}, [2]float32{pos[nj.Name].X, pos[nj.Name].Y})
			if d < 4 {
				t.Errorf("%s and %s settled %.2f apart, want visibly separated", ni.Name, nj.Name, d)
			}
		}
	}
}
