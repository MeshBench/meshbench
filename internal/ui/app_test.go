package ui_test

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/scenario"
	"github.com/MeshBench/meshbench/internal/ui"
)

type flat struct{ h float64 }

func (f flat) ElevationM(_, _ float64) (float64, bool) { return f.h, true }

type noTerrain struct{}

func (noTerrain) ElevationM(_, _ float64) (float64, bool) { return 0, false }

// Selection is the only stateful thing in the shell, and the drawing around it
// cannot be tested at all — so it is worth testing on its own.
func TestSelectionBuildsALink(t *testing.T) {
	a := ui.New(flat{200})

	a.SelectNode(0, false)
	if from, to := a.Link(); from != 0 || to != -1 {
		t.Fatalf("first click gave (%d,%d), want (0,-1)", from, to)
	}
	if a.CutThrough() != nil {
		t.Error("one node is not a path, but an analysis was produced")
	}

	a.SelectNode(1, true)
	if from, to := a.Link(); from != 0 || to != 1 {
		t.Fatalf("ctrl-click gave (%d,%d), want (0,1)", from, to)
	}
	if a.CutThrough() == nil {
		t.Fatalf("two nodes selected but no analysis: %s", a.Error())
	}

	// A plain click starts over rather than extending, or a user can never
	// escape a link once they have made one.
	a.SelectNode(2, false)
	if from, to := a.Link(); from != 2 || to != -1 {
		t.Errorf("a plain click gave (%d,%d), want (2,-1)", from, to)
	}
	if a.CutThrough() != nil {
		t.Error("the previous analysis survived a new selection")
	}
}

// Ctrl-clicking the node already selected must not make a link from a node to
// itself, which has no defined path loss and would render as a division by zero.
func TestCannotLinkANodeToItself(t *testing.T) {
	a := ui.New(flat{200})
	a.SelectNode(0, false)
	a.SelectNode(0, true)
	if _, to := a.Link(); to != -1 {
		t.Errorf("a node was linked to itself (to=%d)", to)
	}
}

// The demo scenario opens on a first run, and a demo that always works teaches
// the wrong thing. This one is a real blocked path over real terrain.
func TestDemoScenarioIsRealAndIncludesAnObserver(t *testing.T) {
	a := ui.New(flat{200})
	if len(a.Nodes) < 3 {
		t.Fatalf("demo has %d nodes", len(a.Nodes))
	}
	var observers, transmitters int
	for _, n := range a.Nodes {
		if err := n.Validate(); err != nil {
			t.Errorf("demo node %s does not validate: %v", n.Name, err)
		}
		if n.Kind == scenario.SDRObserver {
			observers++
		}
		if n.Kind.Transmits() {
			transmitters++
		}
	}
	if observers == 0 {
		t.Error("the demo has no SDR observer, so the node type is invisible on a first run")
	}
	if transmitters < 2 {
		t.Error("the demo cannot form a link")
	}
}

// An observer transmits nothing, so it can never be one end of a link budget.
// Computing a number for it would be a number about a node that cannot talk.
func TestObserverIsNotALinkEnd(t *testing.T) {
	a := ui.New(flat{200})
	observer := -1
	for i, n := range a.Nodes {
		if n.Kind == scenario.SDRObserver {
			observer = i
			break
		}
	}
	if observer < 0 {
		t.Skip("no observer in the demo")
	}
	a.SelectNode(0, false)
	a.SelectNode(observer, true)
	// The analysis still runs — the geometry is real — but the panel refuses to
	// price it. What must not happen is the selection being silently ignored.
	if _, to := a.Link(); to != observer {
		t.Error("selecting an observer as the far end was silently dropped")
	}
}

// No terrain is a reported error, not a crash and not a silent zero.
func TestMissingTerrainIsReported(t *testing.T) {
	a := ui.New(noTerrain{})
	a.SelectNode(0, false)
	a.SelectNode(1, true)
	if a.CutThrough() != nil {
		t.Error("an analysis was produced with no terrain at all")
	}
	if !strings.Contains(a.Error(), "terrain") {
		t.Errorf("the error does not say what is missing: %q", a.Error())
	}
}

// An out-of-range index must leave the selection exactly as it was. Asserting
// it becomes -1 would be wrong now that the app opens on a worked example: the
// requirement is that nothing changes, not that everything is cleared.
func TestOutOfRangeSelectionChangesNothing(t *testing.T) {
	a := ui.New(flat{200})
	beforeFrom, beforeTo := a.Link()

	a.SelectNode(99, false)
	a.SelectNode(-1, true)

	afterFrom, afterTo := a.Link()
	if afterFrom != beforeFrom || afterTo != beforeTo {
		t.Errorf("an out-of-range selection changed (%d,%d) to (%d,%d)",
			beforeFrom, beforeTo, afterFrom, afterTo)
	}
}

// The app opens on a link rather than an empty panel, so the first thing a user
// sees is an answer they can interrogate.
func TestOpensOnAWorkedExample(t *testing.T) {
	a := ui.New(flat{200})
	from, to := a.Link()
	if from < 0 || to < 0 {
		t.Fatalf("opened with no link selected: (%d,%d)", from, to)
	}
	if !a.Nodes[from].Kind.Transmits() || !a.Nodes[to].Kind.Transmits() {
		t.Error("the opening link includes a node that transmits nothing")
	}
	if a.CutThrough() == nil {
		t.Errorf("no analysis on the opening screen: %s", a.Error())
	}
}
