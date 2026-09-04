// What each menu item does.
package workbench

import (
	"context"
	"fmt"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/shell"
	"github.com/MeshBench/meshbench/internal/ui/workbench/nodeview"
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
	nodes    *nodeview.Panel
	chooser  func(string, []string, func(string))
	menuFlag *string
	onShown  func(action string) bool
	// refresh rebuilds the menus, so the ticks follow what docking changed.
	refresh func()
	// wins is the pop-out windows, for the entries that act on all of them.
	wins *panelPopouts
}

// say puts a line in the status bar. A menu entry that declines has to say so
// or it reads as a dead row.
func (w menuDeps) say(line string) {
	go func() { _, _ = w.st.Do(w.ctx, "ui.said", line) }()
}

// runVerb dispatches a verb and puts any refusal in the status bar.
func (w menuDeps) runVerb(verb string) { w.runVerbWith(verb, nil) }

func (w menuDeps) runVerbWith(verb string, params any) {
	go func() {
		if _, err := w.st.Do(w.ctx, verb, params); err != nil {
			_, _ = w.st.Do(w.ctx, "ui.said", verb+": "+err.Error())
		}
	}()
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
	// Starting again with nothing, which throws away whatever is loaded.
	//
	// Asked first, because the network on screen may be an hour of placing
	// and dragging that nothing else can get back - and only asked when
	// there is something to lose, since confirming the discard of an empty
	// network teaches people to dismiss the question without reading it.
	if action == "project.new" {
		// Where, so the map opens somewhere rather than on the middle of the
		// Atlantic: a network with no nodes has nothing to frame a camera on.
		// Blank leaves the view where it was, which is what somebody
		// starting again in the same place wants.
		// Typed, then searched, then chosen from what came back: "Fife"
		// matches two places and "Perth" matches one in Scotland and one in
		// Australia, so the name alone does not say which. Blank starts
		// blank where the map already is.
		askWhere := func() {
			w.sh.Ask.Open("Start a blank network where?",
				"a place: Fife, Perth and Kinross, France - or blank to stay here",
				"", func(place string) {
					place = strings.TrimSpace(place)
					if place == "" {
						w.runVerb("project.new")
						return
					}
					go func() {
						res, err := w.st.Do(w.ctx, "boundary.set",
							map[string]any{"query": place})
						if err != nil {
							w.say("no place called " + place)
							return
						}
						m, _ := res.(map[string]any)
						names, _ := m["names"].([]string)
						if len(names) == 0 {
							w.say("nothing with an outline matches " + place)
							return
						}
						w.sh.Ask.Post(func(ask *shell.Prompt) {
							ask.Choose("Start a blank network over which?", "filter",
								names, func(pick string) {
									w.runVerbWith("project.new",
										map[string]any{"place": pick})
								})
						})
					}()
				})
		}
		snap := w.st.Snapshot()
		if snap == nil || len(snap.Nodes) == 0 {
			askWhere()
			return
		}
		// Asked first, because the network on screen may be an hour of
		// placing and dragging that nothing else can get back.
		w.sh.Ask.Post(func(ask *shell.Prompt) {
			ask.Choose(fmt.Sprintf("Start a blank network? %d nodes are loaded",
				len(snap.Nodes)), "filter",
				[]string{"Start blank, discarding them", "Keep this network"},
				func(pick string) {
					if strings.HasPrefix(pick, "Start blank") {
						askWhere()
					}
				})
		})
		return
	}
	// Opening a network: the names are already known, so the question offers
	// them rather than asking for a path. Before this the entry opened the
	// live-import panel, which is a different thing that happens to also
	// produce w.nodes.
	//
	// The shipped networks are in the list beside the saved ones, because on
	// a fresh install there are no saved ones and the first thing anybody is
	// told to do is open one of the shipped ones.
	if action == "project.open" {
		go func() {
			res, err := w.st.Do(w.ctx, "project.list", nil)
			if err != nil {
				w.say("project.list: " + err.Error())
				return
			}
			names, opens := openChoices(res)
			w.sh.Ask.Post(func(ask *shell.Prompt) {
				ask.Choose("Open a network", "filter - the ones marked built in ship with MeshBench",
					names, func(pick string) {
						what, ok := opens[pick]
						if !ok {
							return
						}
						go func() {
							if _, err := w.st.Do(w.ctx, "project.open", what); err != nil {
								_, _ = w.st.Do(w.ctx, "ui.said", err.Error())
							}
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
