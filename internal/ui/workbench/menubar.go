// Building the menu bar: which menus exist, what is in the Window menu, and
// which one a flag asks to be open at startup.
package workbench

import (
	"context"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/shell"
	"github.com/MeshBench/meshbench/internal/ui/workbench/nodeview"
)

// menuBar is what building the menus needs.
type menuBar struct {
	sh       *shell.Shell
	sets     *settings
	cfg      *configPanel
	dropFlag *string
	st       *state.Store
	ctx      context.Context
	nodes    *nodeview.Panel
	wins     *panelPopouts
	chooser  func(string, []string, func(string))
	menuFlag *string
	// onShown handles the menu entries whose whole point is the map: it
	// flips layers on, and for the actions that need the viewport it
	// dispatches itself and returns true - a raster computed behind a
	// layer that is off is the click that "did nothing".
	onShown func(action string) bool
}

// build fills the menu bar in.
func (b menuBar) build() {
	// The interface's own settings live on the Configuration page now, under
	// Interface - a Settings panel beside a Configuration page was two homes
	// for one kind of thing.
	b.cfg.sets = b.sets
	// File is where a session begins and ends, and it had one item in it.
	//
	// Firmware lives here rather than under Repeaters because a companion, a
	// room server and an SDR observer all run firmware too - filing it under
	// one node type is how somebody looking for a companion build never finds
	// it.
	// Each menu is its own verbs plus the panels that named it, so a menu is
	// read as one list: what this part of the application can do, and what it
	// can show. Rebuilt every frame in Refresh, because the ticks move.
	b.refresh()
	if *b.dropFlag != "" {
		// Before the frame loop starts, so no goroutine races the renderer.
		b.sh.OpenMenu(*b.dropFlag)
	}
	b.sh.OnMenu = menuDeps{
		sh: b.sh, st: b.st, ctx: b.ctx, cfg: b.cfg, nodes: b.nodes,
		chooser: b.chooser, menuFlag: b.menuFlag, onShown: b.onShown,
		refresh: b.refresh, wins: b.wins,
	}.onMenu
}

// refresh rebuilds every menu from the table plus the panels that named it.
//
// Called again after anything that changes what is on screen, because the
// ticks are the menu's answer to "what am I looking at" and a stale tick is a
// menu that lies.
func (b menuBar) refresh() {
	for _, m := range workbenchMenus() {
		b.sh.SetMenu(m.Name, mergeSections(m.Items, b.sh.PanelItems(m.Name)))
	}
}

// mergeSections folds the panel rows into the menu's own sections.
//
// Appended instead, a menu grew a second "Open & Save" heading below Quit,
// because the dropdown draws a heading wherever the section changes and the
// panels came after everything. A section is one place in a menu.
func mergeSections(items, panels []shell.MenuItem) []shell.MenuItem {
	out := append([]shell.MenuItem{}, items...)
	for _, it := range panels {
		last := -1
		for i := range out {
			if out[i].Section == it.Section {
				last = i
			}
		}
		if last < 0 {
			out = append(out, it)
			continue
		}
		out = append(out[:last+1], append([]shell.MenuItem{it}, out[last+1:]...)...)
	}
	return out
}
