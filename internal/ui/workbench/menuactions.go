// What each menu item does.
package workbench

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/shell"
)

// menuDeps is what a menu action can reach.
//
// The receiver is w rather than m because one of the actions unpacks a verb
// result into a local called m, and a shadowed receiver compiles into
// something that looks like a field on a map.
type menuDeps struct {
	sh       *shell.Shell
	st       *state.Store
	ctx      context.Context
	cfg      *configPanel
	nodes    *nodesPanel
	chooser  func(string, []string, func(string))
	menuFlag *string
	onShown  func(action string) bool
	// refresh rebuilds the menus, so the ticks follow what docking changed.
	refresh func()
	// wins is the pop-out windows, for the entries that act on all of them.
	wins *windows
}

// say puts a line in the status bar. A menu entry that declines has to say so
// or it reads as a dead row.
func (w menuDeps) say(line string) {
	go func() { _, _ = w.st.Do(w.ctx, "ui.said", line) }()
}

// onMenu is what every menu item does.
//
// One switch rather than a handler per item: the shell hands back the action
// string a menu was built with, so the item and the thing it does are named
// the same in one place, and a menu entry that reaches nothing is a case that
// is missing rather than a wiring mistake somewhere else.
func (w menuDeps) onMenu(action string) {
	// A panel entry toggles the panel in the layout: docked here, in this
	// window, where it can be seen beside everything else. It used to throw
	// the panel out into an OS window that could not be raised on Linux and
	// had no route back, which is how "show all panels" came to hide them.
	if name, ok := strings.CutPrefix(action, "panel."); ok {
		if w.sh.Visible(name) {
			w.sh.Undock(name)
		} else {
			w.sh.Dock(name)
		}
		if w.refresh != nil {
			w.refresh()
		}
		return
	}
	// Sending a panel to a window of its own, which is now the secondary way
	// to see one rather than the only way.
	if name, ok := strings.CutPrefix(action, "window.panel."); ok {
		if w.sh.OnPopOut != nil {
			w.sh.OnPopOut(name)
		}
		return
	}
	if action == "layout.reset" {
		w.sh.ResetLayout(w.sh.View)
		if w.refresh != nil {
			w.refresh()
		}
		return
	}
	// What to do with the windows that have left: bring them where they can
	// be seen, or bring them home. Both say what happened, because with no
	// windows open either one is otherwise a press with no effect.
	if action == "window.raise_all" || action == "window.dock_all" {
		names := w.wins.names()
		if len(names) == 0 {
			w.say("no panel is in a window of its own")
			return
		}
		for _, n := range names {
			if action == "window.dock_all" {
				w.wins.dock(n)
				w.sh.Dock(n)
			} else {
				w.wins.raise(n)
			}
		}
		if action == "window.dock_all" {
			w.say(fmt.Sprintf("docked %d back", len(names)))
			if w.refresh != nil {
				w.refresh()
			}
		}
		return
	}
	// Opening one of your own networks: the names are already known, so the
	// question offers them rather than asking for a path. Before this the
	// entry opened the live-import panel, which is a different thing that
	// happens to also produce w.nodes.
	if action == "project.open" {
		go func() {
			res, err := w.st.Do(w.ctx, "project.list", nil)
			if err != nil {
				return
			}
			m, _ := res.(map[string]any)
			dir, _ := m["dir"].(string)
			var names []string
			if raw, ok := m["projects"].([]string); ok {
				names = raw
			}
			w.sh.Ask.Post(func(ask *shell.Prompt) {
				ask.Choose("Open a saved network", "filter", names,
					func(name string) {
						go func() {
							_, _ = w.st.Do(w.ctx, "project.open",
								filepath.Join(dir, name+".json"))
						}()
					})
			})
		}()
		return
	}
	// Play, when the w.nodes have no build to run.
	//
	// The verb refuses, correctly - a run with half a mesh up measures a
	// network that does not exist. But the refusal named 34 w.nodes and left
	// the operator to go and pin them, which is not a thing anybody does one
	// node at a time. Ask by role instead, which is how firmware is chosen
	// anyway: one answer covers every repeater.
	if action == "sim.start" {
		go func() {
			res, err := w.st.Do(w.ctx, "firmware.needed", nil)
			if err == nil {
				if roles := rolesNeeding(res); len(roles) > 0 {
					askForFirmware(w.ctx, w.st, &w.sh.Ask, roles)
					return
				}
			}
			if _, err := w.st.Do(w.ctx, "sim.start", nil); err != nil {
				_, _ = w.st.Do(w.ctx, "ui.said", err.Error())
			}
		}()
		return
	}
	// Planning between two w.nodes, when two are not already selected.
	//
	// The verb reads the selection, so from a menu it simply refused, and
	// "select two w.nodes to plan between" in a status bar is a rebuke rather
	// than an instruction. Asking for the ends is the instruction.
	if action == "plan.routes" {
		if sel := selectedNames(w.st.Snapshot()); len(sel) < 2 {
			askForRouteEnds(w.ctx, w.st, &w.sh.Ask, sel)
			return
		}
	}
	// Verbs that need a word from the operator. A menu entry carries no
	// parameters, so before this the item fired, the verb refused, and the
	// only trace was a line in the status bar nobody was reading.
	if ask, ok := menuAsks[action]; ok {
		w.sh.Ask.Open(ask.title, ask.hint, ask.initial(), func(answer string) {
			if strings.TrimSpace(answer) == "" {
				return
			}
			go func() {
				if _, err := w.st.Do(w.ctx, action, map[string]any{ask.field: answer}); err != nil {
					_, _ = w.st.Do(w.ctx, "ui.said", err.Error())
				}
			}()
		})
		return
	}
	// The interface's own settings are a Configuration section now.
	// Both of these are questions the Configuration page answers, opened on
	// the section that answers them.
	if section, ok := map[string]string{
		"config.interface": "Interface",
		"help.assumptions": "RF Simulation",
	}[action]; ok {
		w.cfg.Open(section)
		w.sh.Dock("Configuration")
		if w.refresh != nil {
			w.refresh()
		}
		return
	}
	if action == "ui.toggle_real_firmware" {
		// Read the current value and send its opposite, so the control
		// never has its own copy of the answer to drift from the store's.
		go func() {
			real := false
			if s := w.st.Snapshot(); s != nil {
				real = s.RealFirmware
			}
			_, _ = w.st.Do(w.ctx, "sim.kind", map[string]any{"real": !real})
		}()
		return
	}
	if w.onShown != nil && w.onShown(action) {
		return
	}
	// The error reaches the status bar. Dropping it made a refusal look
	// like a dead button: sim.start declining to run half a mesh - two
	// placed repeaters with no firmware pinned - answered a play press
	// with absolute silence.
	go func() {
		if _, err := w.st.Do(w.ctx, action, nil); err != nil {
			_, _ = w.st.Do(w.ctx, "ui.said", err.Error())
		}
	}()
}
