package workbench

import (
	"fmt"
	"image"
	"sort"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/shell"
	"github.com/MeshBench/meshbench/internal/ui/theme"
	"github.com/MeshBench/meshbench/internal/ui/uitest"
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
			text.NewShaper(text.WithCollection(uitest.BrandFaces()))),
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
	//
	// Taken from the same table the registrations use, so a panel added to
	// the application cannot be missing from the walkthrough - the old hand
	// written list here was already three panels short of the real one.
	for n := range panelMenus {
		h.sh.Add(homed(shell.EmptyPanel(n, "for the walkthrough")))
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

// Every panel is one click from a menu, and named in exactly one of them.
//
// This is what replaced "Show all panels...": that entry was a chooser of all
// thirty-three, and picking one threw it out of the window. A panel nobody
// can find from a menu is a panel that does not exist, and a panel in two
// menus is two entries to keep in step - one of which is always the stale one.
func TestEveryPanelIsInExactlyOneMenu(t *testing.T) {
	h := newShellHarness(t)
	menus := map[string][]shell.MenuItem{}
	for _, m := range workbenchMenus() {
		menus[m.Name] = m.Items
	}
	seen := map[string][]string{}
	for _, name := range shell.MenuNames {
		items := append(append([]shell.MenuItem{}, menus[name]...), h.sh.PanelItems(name)...)
		h.sh.SetMenu(name, items)
		for _, it := range items {
			if p, ok := strings.CutPrefix(it.Action, "panel."); ok {
				seen[p] = append(seen[p], name)
			}
		}
	}
	for name := range h.sh.Panels {
		switch len(seen[name]) {
		case 1:
		case 0:
			t.Errorf("%q is in no menu, so nothing can open it", name)
		default:
			t.Errorf("%q is in %v; one entry lives in one menu", name, seen[name])
		}
	}
	// And show-all is gone rather than merely unused.
	for _, name := range shell.MenuNames {
		for _, it := range h.sh.MenuItems(name) {
			if it.Action == "window.showall" {
				t.Errorf("%s still offers the show-all chooser", name)
			}
		}
	}
}

// A menu entry for a panel docks it into the layout, and says so with a tick.
func TestPanelEntryDocksAndTicks(t *testing.T) {
	h := newShellHarness(t)
	h.frame()
	const name = "Waterfall"
	if h.sh.Visible(name) {
		t.Fatalf("%s is in the Plan view already; pick a panel that is not", name)
	}
	iconFor := func() string {
		for _, it := range h.sh.PanelItems("Analysis") {
			if it.Action == "panel."+name {
				return it.Icon
			}
		}
		return ""
	}
	if got := iconFor(); got != "panel" {
		t.Fatalf("a panel that is not on screen carries the %q glyph, want the plain panel outline", got)
	}
	h.sh.Dock(name)
	if !h.sh.Visible(name) {
		t.Fatal("docking a panel did not put it in the layout")
	}
	if got := iconFor(); got != "tick" {
		t.Fatalf("a docked panel carries the %q glyph, want a tick", got)
	}
	h.frame()
	// And it draws where it was docked, rather than in a window elsewhere.
	if !containsPanel(h.sh.VisiblePanels(), name) {
		t.Fatal("the docked panel is not among the panels this view is showing")
	}
	h.sh.Undock(name)
	if h.sh.Visible(name) {
		t.Fatal("undocking left the panel in the layout")
	}
}

func containsPanel(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
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
