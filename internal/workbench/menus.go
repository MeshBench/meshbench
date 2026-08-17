// The menu bar, and the small conversions the shell needs on the way to
// drawing a node.
package workbench

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gioui.org/font"
	"gioui.org/font/opentype"
	"github.com/MeshBench/meshbench/internal/gui/shell"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
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
			{Label: "Open a saved network", Action: "project.open",
				Section: "Open & Save", Icon: "folder", Shortcut: "Ctrl+O"},
			{Label: "Save this network", Action: "project.save",
				Section: "Open & Save", Icon: "save", Shortcut: "Ctrl+S"},
			{Label: "Save this run", Action: "run.save",
				Section: "Open & Save", Icon: "save", Shortcut: "Ctrl+Shift+S"},
			{Label: "Firmware library", Action: "panel.Firmware",
				Section: "Import & Export", Icon: "chip"},
			{Label: "Import a live network", Action: "panel.Import",
				Section: "Import & Export", Icon: "import"},
			{Label: "Export the event log", Action: "events.dump",
				Section: "Import & Export", Icon: "export"},
			{Label: "Quit", Action: "app.quit",
				Section: "Exit", Icon: "exit", Shortcut: "Ctrl+Q"},
		}},
		{"View", []shell.MenuItem{
			{Label: "Nodes running", Action: "panel.Nodes running",
				Section: "Overview", Icon: "nodes"},
			{Label: "Companion bench", Action: "panel.Companion bench",
				Section: "Overview", Icon: "chip"},
			{Label: "Experiment log", Action: "panel.Experiment log",
				Section: "Diagnostics", Icon: "log"},
			{Label: "Configuration", Action: "panel.Configuration",
				Section: "Diagnostics", Icon: "sliders"},
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
			{Label: "Capture the waterfall", Action: "waterfall.capture",
				Section: "Tools", Icon: "waterfall"},
			{Label: "Capture to a pcapng file", Action: "capture.file",
				Section: "Tools", Icon: "pcap"},
		}},
		{"Repeaters", []shell.MenuItem{
			{Label: "Send a command to the fleet", Action: "panel.Fleet",
				Section: "Commands", Icon: "send"},
			{Label: "What they are told at boot", Action: "panel.Provisioning",
				Section: "Boot", Icon: "boot"},
			{Label: "Coverage from the selection", Action: "coverage.compute",
				Section: "Analysis", Icon: "coverage"},
		}},
		// Import is under File, with opening and saving, and not here as
		// well: one entry in two menus is two entries to keep in step and one
		// of them is always the stale one.
		{"Planning", []shell.MenuItem{
			{Label: "Routes between two selected nodes", Action: "plan.routes",
				Section: "Tools", Icon: "route"},
			{Label: "Boundary", Action: "panel.Boundary",
				Section: "Tools", Icon: "boundary"},
		}},
		// Window is generated from the panels themselves, so it is not here.
		{"Help", []shell.MenuItem{
			{Label: "What this run assumes", Action: "panel.Configuration",
				Icon: "help"},
			{Label: "Licences & attributions", Action: "panel.Licences",
				Icon: "doc"},
		}},
	}
}

func withEmoji(base []font.FontFace) []font.FontFace {
	// The bundle carries the font beside the binary, so emoji in node names
	// do not depend on what the machine happens to have installed.
	paths := []string{
		"/usr/share/fonts/noto/NotoColorEmoji.ttf",
		"/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf",
		"/System/Library/Fonts/Apple Color Emoji.ttc",
	}
	if exe, err := os.Executable(); err == nil {
		paths = append([]string{
			filepath.Join(filepath.Dir(exe), "fonts", "NotoColorEmoji.ttf"),
		}, paths...)
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if faces, err := opentype.ParseCollection(b); err == nil {
			return append(base, faces...)
		}
	}
	return base
}

func shortKind(k string) string {
	switch k {
	case "simple-repeater":
		return "repeater"
	case "advanced-repeater":
		return "advanced"
	case "sdr-observer":
		return "observer"
	case "room-server":
		return "room server"
	}
	return k
}

func kindOf(k string) theme.NodeKind {
	switch k {
	case "companion":
		return theme.Companion
	case "room-server":
		return theme.RoomServer
	case "sdr-observer":
		return theme.Observer
	case "emitter":
		return theme.Emitter
	case "advanced-repeater":
		return theme.AdvancedRepeater
	}
	return theme.SimpleRepeater
}

func itoa(i int) string { return fmt.Sprintf("%d", i) }

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
