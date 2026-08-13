package main

import (
	"fmt"
	"image"
	"sort"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/font/gofont"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/A13xB0/meshcoresim/internal/gui/shell"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// Does every menu item do something?
//
// Not "does the verb exist" and not "does the handler fire when called" - does
// pressing the thing, where it actually is, in a real layout, reach anything
// at all. A menu entry whose action nobody handles is silent, and silence is
// indistinguishable from a slow machine.
//
// This is the test that should have existed from the start. The map's node
// menu was dispatching a verb no switch case handled, and every other check in
// this repository passed while it did.

type shellHarness struct {
	sh   *shell.Shell
	th   *theme.Theme
	r    input.Router
	ops  op.Ops
	sz   image.Point
	snap *state.Snapshot
	// fired records every action the shell asked for.
	fired []string
}

func newShellHarness(t *testing.T) *shellHarness {
	t.Helper()
	h := &shellHarness{
		sh: shell.New(),
		th: theme.New(theme.Dark, theme.Default,
			text.NewShaper(text.WithCollection(gofont.Collection()))),
		sz: image.Pt(1700, 1000),
		snap: &state.Snapshot{
			Nodes: []state.Node{
				{Name: "Abernethy Repeater", Kind: "simple-repeater", Selected: true},
				{Name: "AngusOutlaw1", Kind: "companion"},
			},
		},
	}
	// Every panel the real workbench registers, as an empty body: this test is
	// about whether the chrome reaches anything, not about what a panel draws.
	for _, n := range []string{
		"Map", "Nodes", "Inspector", "Events", "Scoreboard", "Packet timeline",
		"Waterfall", "Link", "Budget", "Planning", "Boundary", "Import",
		"Validate", "Energy", "Live feed", "Console", "Schedule", "Compare",
		"Sweep", "Runs", "Matrix", "Timelines", "Experiment log",
		"Configuration", "Firmware", "Fleet", "Companion bench",
		"Nodes running", "Settings", "Provisioning",
	} {
		h.sh.Add(shell.EmptyPanel(n, "for the walkthrough"))
	}
	h.sh.OnMenu = func(action string) { h.fired = append(h.fired, action) }
	h.sh.OnPopOut = func(name string) { h.fired = append(h.fired, "panel."+name) }
	h.sh.PoppedOut = func(string) bool { return false }
	return h
}

func (h *shellHarness) frame() {
	h.ops.Reset()
	gtx := layout.Context{
		Ops:         &h.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(h.sz),
		Source:      h.r.Source(),
	}
	h.sh.Layout(h.th, gtx, h.snap)
	h.r.Frame(&h.ops)
}

func (h *shellHarness) click(at f32.Point) {
	h.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: at, Buttons: pointer.ButtonPrimary},
		pointer.Event{Kind: pointer.Release, Position: at, Buttons: pointer.ButtonPrimary},
	)
	h.frame()
}

// TestEveryMenuItemReachesSomething opens each menu and presses every entry.
func TestEveryMenuItemReachesSomething(t *testing.T) {
	h := newShellHarness(t)
	// The menus the workbench sets, with the items it sets on them.
	menus := map[string][]shell.MenuItem{
		"File": {
			{Label: "Open a saved network", Action: "panel.Import"},
			{Label: "Save this network", Action: "project.save"},
			{Label: "Save this run", Action: "run.save"},
			{Label: "Firmware library", Action: "panel.Firmware"},
			{Label: "Import a live network", Action: "panel.Import"},
			{Label: "Export the event log", Action: "events.dump"},
			{Label: "Quit", Action: "app.quit"},
		},
		"Simulation": {
			{Label: "Play or pause", Action: "sim.start"},
			{Label: "One step", Action: "sim.step"},
			{Label: "Back to the start", Action: "sim.reset"},
			{Label: "Start firmware on every node", Action: "firmware.start"},
			{Label: "Wipe every node's memory", Action: "firmware.wipe"},
			{Label: "Originate a packet", Action: "sim.inject"},
			{Label: "Capture the waterfall", Action: "waterfall.capture"},
			{Label: "Capture to a pcapng file", Action: "capture.file"},
		},
	}
	for name, items := range menus {
		h.sh.SetMenu(name, items)
	}
	h.frame()

	dead := map[string][]string{}
	for name, items := range menus {
		for i := range items {
			before := len(h.fired)
			// Open the menu, then press the i-th entry under it.
			h.clickMenu(name)
			h.clickMenuItem(name, i)
			if len(h.fired) == before {
				dead[name] = append(dead[name], items[i].Label)
			}
		}
	}
	if len(dead) > 0 {
		var lines []string
		for menu, items := range dead {
			lines = append(lines, fmt.Sprintf("%s: %s", menu, strings.Join(items, ", ")))
		}
		sort.Strings(lines)
		t.Errorf("menu entries that reach nothing:\n  %s", strings.Join(lines, "\n  "))
	}
	t.Logf("%d actions fired: %v", len(h.fired), h.fired)
}

// clickMenu opens a named menu by pressing its title in the bar.
func (h *shellHarness) clickMenu(name string) {
	x := h.sh.MenuX(name)
	if x < 0 {
		return
	}
	h.click(f32.Pt(float32(x)+6, 10))
}

// clickMenuItem presses the i-th entry of the open menu.
func (h *shellHarness) clickMenuItem(menu string, i int) {
	x := h.sh.MenuX(menu)
	if x < 0 {
		return
	}
	// Entries stack below the bar; the bar is one row.
	y := float32(h.th.RowHeight())*1.0 + float32(i)*22 + 11
	h.click(f32.Pt(float32(x)+20, y))
}
