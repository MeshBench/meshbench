// The menu bar, and the small conversions the shell needs on the way to
// drawing a node.
package workbench

import (
	"fmt"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/shell"
)

// workbenchMenus is the menu bar, in one place.
//
// It used to be a run of SetMenu calls, with the test that presses every entry
// keeping its own copy. The copy drifted - it was still pressing the entry that
// opened the import panel after that entry had become "open a saved network" -
// so the test passed while checking a menu bar nobody had.
// The sections, icons and shortcuts are the UX design's, verbatim: grouped by
// purpose under headings, the most common action first in each menu, and the
// binding shown in the row is the binding the shell registers - one table
// feeds both, so they cannot drift apart.
func workbenchMenus() []menu {
	return []menu{
		{"File", []shell.MenuItem{
			{Label: "New blank network", Action: "project.new",
				Section: "Open & Save", Icon: "doc", Shortcut: "Ctrl+N"},
			{Label: "Open a saved network", Action: "project.open",
				Section: "Open & Save", Icon: "folder", Shortcut: "Ctrl+O"},
			{Label: "Save this network", Action: "project.save",
				Section: "Open & Save", Icon: "save", Shortcut: "Ctrl+S"},
			{Label: "Save this run", Action: "run.save",
				Section: "Open & Save", Icon: "save", Shortcut: "Ctrl+Shift+S"},
			{Label: "Export the event log", Action: "events.dump",
				Section: "Import & Export", Icon: "export"},
			{Label: "Quit", Action: "app.quit",
				Section: "Exit", Icon: "exit", Shortcut: "Ctrl+Q"},
		}},
		{"View", []shell.MenuItem{
			{Label: "Settings", Action: "config.interface",
				Section: "Preferences", Icon: "settings"},
		}},
		{"Simulation", []shell.MenuItem{
			{Label: "Play or pause", Action: "sim.start",
				Section: "Control", Icon: "play", Shortcut: "Space"},
			{Label: "One step", Action: "sim.step",
				Section: "Control", Icon: "step"},
			{Label: "Back to the start", Action: "sim.reset",
				Section: "Control", Icon: "back"},
			{Label: "Start firmware on every node", Action: "firmware.start",
				Section: "Nodes", Icon: "power"},
			{Label: "Wipe every node's memory", Action: "firmware.wipe",
				Section: "Nodes", Icon: "wipe"},
			{Label: "Originate a packet", Action: "sim.inject",
				Section: "Tools", Icon: "packet"},
			{Label: "Coverage raster of the whole map", Action: "coverage.map",
				Section: "Tools", Icon: "coverage"},
			{Label: "Coverage raster of this view", Action: "coverage.viewport",
				Section: "Tools", Icon: "coverage"},
			{Label: "Capture the waterfall", Action: "waterfall.capture",
				Section: "Tools", Icon: "waterfall"},
			{Label: "Capture to a pcapng file", Action: "capture.file",
				Section: "Tools", Icon: "pcap"},
			// Live, and one item rather than three: streaming, installing the
			// dissector and opening Wireshark are one thing to an operator,
			// and asking them to do the other two by hand is how this stopped
			// being usable in the first place.
			{Label: "Watch it live in Wireshark", Action: "capture.wireshark",
				Section: "Tools", Icon: "pcap"},
			{Label: "Stop capturing", Action: "capture.stop",
				Section: "Tools", Icon: "pcap"},
		}},
		// Mesh rather than Repeaters: a companion, a room server and an SDR
		// observer are all nodes, and filing their panels under one node type
		// is how somebody looking for the companion bench never finds it.
		{"Mesh", []shell.MenuItem{
			{Label: "Coverage from the selection", Action: "coverage.compute",
				Section: "Analysis", Icon: "coverage"},
			{Label: "Coverage from the selection, this view", Action: "coverage.selection.viewport",
				Section: "Analysis", Icon: "coverage"},
		}},
		// Analysis is new. The study panels - the link cut-through, the
		// budget, the matrix, validation - had no menu of their own, which is
		// most of what could previously only be reached by a chooser.
		{"Analysis", []shell.MenuItem{
			{Label: "Routes between two selected nodes", Action: "plan.routes",
				Section: "Tools", Icon: "route"},
		}},
		// Window is about windows and layouts; the panels themselves are
		// listed in the menu each one belongs to.
		{"Window", []shell.MenuItem{
			{Label: "Reset this view's layout", Action: "layout.reset",
				Section: "Layout", Icon: "grid"},
			{Label: "Bring every window to the front", Action: "window.raise_all",
				Section: "Windows", Icon: "window"},
			{Label: "Dock every window back", Action: "window.dock_all",
				Section: "Windows", Icon: "panel"},
		}},
		// A question, not a second door to a panel: it opens Configuration on
		// the section that answers it. Pointing this at panel.Configuration
		// gave that panel two menu homes, which is two entries to keep in
		// step and one of them always stale.
		{"Help", []shell.MenuItem{
			{Label: "What this run assumes", Action: "help.assumptions",
				Icon: "help"},
		}},
	}
}

// nextPlacedName is a name nothing else has, in the kind's own words.
func nextPlacedName(kind string, s *state.Snapshot) string {
	base := strings.ReplaceAll(kind, "simple-", "")
	base = strings.ReplaceAll(base, "-", " ")
	taken := map[string]bool{}
	for i := range s.Nodes {
		taken[s.Nodes[i].Name] = true
	}
	for n := 1; ; n++ {
		name := fmt.Sprintf("new %s %d", base, n)
		if !taken[name] {
			return name
		}
	}
}
