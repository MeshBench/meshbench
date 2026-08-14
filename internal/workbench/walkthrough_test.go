package workbench

import (
	"fmt"
	"image"
	"sort"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/font/gofont"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/gui/shell"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
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
		"Nodes running", "Provisioning", "Packet",
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
	// The workbench's own menu bar, not a copy of it.
	menus := map[string][]shell.MenuItem{}
	for _, m := range workbenchMenus() {
		menus[m.Name] = m.Items
	}
	for name, items := range menus {
		h.sh.SetMenu(name, items)
	}
	h.frame()

	dead := map[string][]string{}
	for name, items := range menus {
		for i := range items {
			// Open the menu, then press the entry where it actually is.
			if !h.fireMenuItem(name, items[i].Action) {
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

// The shortcut in the row is the shortcut that works: one table feeds the
// caption and the key filter, and this presses the keys.
func TestMenuShortcutsFire(t *testing.T) {
	h := newShellHarness(t)
	for _, m := range workbenchMenus() {
		h.sh.SetMenu(m.Name, m.Items)
	}
	h.frame()
	cases := []struct {
		name key.Name
		mods key.Modifiers
		want string
	}{
		{"O", key.ModCtrl, "project.open"},
		{"S", key.ModCtrl, "project.save"},
		{"S", key.ModCtrl | key.ModShift, "run.save"},
		{"Q", key.ModCtrl, "app.quit"},
		{key.NameSpace, 0, "sim.start"},
	}
	for _, c := range cases {
		before := len(h.fired)
		h.r.Queue(key.Event{Name: c.name, Modifiers: c.mods, State: key.Press})
		h.frame()
		if len(h.fired) == before {
			t.Errorf("%v+%v fired nothing, want %s", c.mods, c.name, c.want)
			continue
		}
		if got := h.fired[len(h.fired)-1]; got != c.want {
			t.Errorf("%v+%v fired %s, want %s", c.mods, c.name, got, c.want)
		}
	}
}

// The Window menu ends in a pinned show-all row, and it can be pressed.
func TestWindowMenuHasShowAll(t *testing.T) {
	h := newShellHarness(t)
	h.sh.WindowMenu("Window")
	h.frame()
	items := h.sh.MenuItems("Window")
	if len(items) == 0 {
		t.Fatal("the Window menu is empty")
	}
	last := items[len(items)-1]
	if last.Action != "window.showall" {
		t.Fatalf("the last Window entry is %q, want the show-all overflow", last.Action)
	}
	if !h.fireMenuItem("Window", "window.showall") {
		t.Error("no pointer position reaches the pinned show-all row")
	}
}

// clickMenu opens a named menu by pressing its title in the bar.
func (h *shellHarness) clickMenu(name string) {
	x := h.sh.MenuX(name)
	if x < 0 {
		return
	}
	h.click(f32.Pt(float32(x)+6, 10))
}

// fireMenuItem presses an entry of a menu by walking a pointer down the open
// dropdown until that entry's action fires.
//
// A walk rather than a computed coordinate: the dropdown now carries section
// headings and rules between groups, so "the i-th entry is at i times the row
// height" is exactly the kind of layout assumption this test exists to not
// make. Hitting a different entry on the way down fires its action and closes
// the menu, so the menu is reopened and the walk continues below.
func (h *shellHarness) fireMenuItem(menu, action string) bool {
	if h.sh.MenuX(menu) < 0 {
		return false
	}
	x := float32(h.sh.MenuX(menu)) + 30
	for y := float32(h.th.RowHeight()) + 4; y < 950; y += 4 {
		if h.sh.OpenMenuIndex() < 0 {
			h.clickMenu(menu)
		}
		before := len(h.fired)
		h.click(f32.Pt(x, y))
		if len(h.fired) > before && h.fired[len(h.fired)-1] == action {
			return true
		}
	}
	return false
}
