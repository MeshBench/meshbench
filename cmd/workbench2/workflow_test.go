package main

import (
	"fmt"
	"strings"
	"testing"

	"gioui.org/f32"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
)

// The workflows, walked by clicking.
//
// Not "the verb works" - can somebody get there by pointing at things. Each
// test records the clicks it needed, and a workflow needing more than a
// handful of them, or needing something typed that is already on screen, is
// reported as such rather than passed.

type walk struct {
	steps []string
}

func (w *walk) step(s string) { w.steps = append(w.steps, s) }

func (w *walk) report(t *testing.T, what string) {
	t.Helper()
	t.Logf("%s - %d steps:", what, len(w.steps))
	for i, s := range w.steps {
		t.Logf("   %d. %s", i+1, s)
	}
}

// Change one node's firmware, from the node table.
func TestWorkflowChangeOneNodesFirmware(t *testing.T) {
	w := &walk{}
	p := &nodeViewPanel{}
	var did []string
	p.OnFirmware = func(node, version string) {
		did = append(did, node+" -> "+version)
	}
	snap := &state.Snapshot{
		Stats: []state.NodeStat{
			{Name: "Abernethy Repeater", Backend: "native", Running: true,
				Firmware: "repeater-v1.17.0", RSSBytes: 4 << 20},
			{Name: "Bishop Hill", Backend: "native", Running: true,
				Firmware: "repeater-v1.17.0", RSSBytes: 4 << 20},
		},
		Builds: []state.Build{
			{Version: "repeater-v1.16.0", Role: "simple_repeater", Native: true},
			{Version: "repeater-v1.17.0", Role: "simple_repeater", Native: true},
		},
	}
	h := newPanelHarness(p.Draw, snap)
	h.frame()

	// 1. Click the firmware cell of the row you want.
	//
	// Scanned rather than computed: where a row sits is the layout's business,
	// and a test that writes the coordinate down is testing its own arithmetic.
	w.step("click the firmware cell on the node's row")
	x := float32(0)
	for _, c := range nodeColumns() {
		if c.Title == "firmware" {
			break
		}
		wpx := c.Width
		if wpx == 0 {
			wpx = 120
		}
		x += float32(wpx)
	}
	for y := float32(40); y < 260 && p.pickFor == ""; y += 6 {
		h.click(f32.Pt(x+40, y))
		h.frame()
	}

	if p.pickFor == "" {
		t.Fatal("clicking the firmware cell opened nothing: there is no way " +
			"to change one node's firmware by pointing at it")
	}
	w.step(fmt.Sprintf("a list of builds opens for %q", p.pickFor))
	t.Logf("builds available to the picker: %d (%v)", len(p.builds), firstFew(p.builds))

	// 2. Click the build you want.
	w.step("click the build to use")
	// From the bottom up: cancel sits at the top of the list, so scanning
	// downward closes the thing before reaching what it offers.
	for y := float32(h.sz.Y) - 20; y > 30 && len(did) == 0; y -= 8 {
		h.pressAlong(y)
	}
	if len(did) == 0 {
		t.Errorf("the build list opened but choosing one reached nothing")
	} else {
		w.step("it is applied: " + did[0])
	}
	w.report(t, "Change one node's firmware")
	if len(w.steps) > 4 {
		t.Errorf("%d steps to change one node's firmware", len(w.steps))
	}
}

// Find a node by name.
func TestWorkflowSearchForANode(t *testing.T) {
	w := &walk{}
	var selected string
	np := &nodesPanel{OnSelect: func(n string) { selected = n }}
	snap := &state.Snapshot{Nodes: []state.Node{
		{Name: "Abernethy Repeater", Kind: "simple-repeater"},
		{Name: "Bishop Hill", Kind: "simple-repeater"},
		{Name: "AngusOutlaw1", Kind: "companion"},
	}}
	h := newPanelHarness(np.Draw, snap)
	h.frame()

	w.step("click the filter box at the top of the Nodes panel")
	h.click(f32.Pt(150, 20))
	w.step("type part of the name")
	h.typeText("bishop")
	h.frame()

	shown := np.tbl.ShownKeys()
	if len(shown) != 1 {
		t.Errorf("typing \"bishop\" leaves %d of 3 nodes", len(shown))
	}
	for k := range shown {
		w.step("the list narrows to " + k)
	}
	// And what a person does next: click it to select it.
	w.step("click the row to select it")
	for y := float32(30); y < 240 && np.tbl.Selected == ""; y += 6 {
		h.click(f32.Pt(150, y))
		h.frame()
	}
	if np.tbl.Selected == "" {
		t.Error("clicking the found row selects nothing, so the search leads nowhere")
	} else {
		w.step("selected: " + np.tbl.Selected)
	}
	// And the rest of the workbench has to hear about it, or the map and the
	// Inspector keep showing a different node than the one just clicked.
	if selected == "" {
		t.Error("the selection never leaves this panel: the map and Inspector " +
			"still show whatever was selected before")
	} else {
		w.step("the map and Inspector follow it")
	}
	w.report(t, "Search for a node")
}

var _ = strings.Contains

func firstFew(v []string) []string {
	if len(v) > 3 {
		return v[:3]
	}
	return v
}
