// Building the menu bar: which menus exist, what is in the Window menu, and
// which one a flag asks to be open at startup.
package workbench

import (
	"context"

	"github.com/MeshBench/meshbench/internal/gui/shell"
	"github.com/MeshBench/meshbench/internal/gui/state"
)

// menuBar is what building the menus needs.
type menuBar struct {
	sh       *shell.Shell
	sets     *settings
	cfg      *configPanel
	dropFlag *string
	st       *state.Store
	ctx      context.Context
	nodes    *nodesPanel
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
	for _, m := range workbenchMenus() {
		b.sh.SetMenu(m.Name, m.Items)
	}
	// The Window menu had become a dumping ground: every panel, alphabetical,
	// thirty rows deep. It lists the daily set; everything else stays one
	// step away behind "Show all panels...".
	for _, name := range []string{
		"Map", "Events", "Nodes", "Nodes running", "Console", "Fleet",
		"Firmware", "Configuration", "Packet timeline", "Packet",
		"Waterfall", "Companion bench", "Experiment log",
	} {
		if p := b.sh.Panels[name]; p != nil {
			p.InWindowMenu = true
		}
	}
	b.sh.WindowMenu("Window")
	if *b.dropFlag != "" {
		// Before the frame loop starts, so no goroutine races the renderer.
		b.sh.OpenMenu(*b.dropFlag)
	}
	b.sh.OnMenu = menuDeps{
		sh: b.sh, st: b.st, ctx: b.ctx, cfg: b.cfg, nodes: b.nodes,
		chooser: b.chooser, menuFlag: b.menuFlag, onShown: b.onShown,
	}.onMenu
}
