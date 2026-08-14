package session

import (
	"context"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/gui/state"
)

// Every menu item, run the way the menu runs it.
//
// The shell fires an action with no parameters, because a menu entry has none
// to give. A verb that needs a node, a name or a path therefore fails the
// moment somebody picks it from the menu - it fires, it errors, and all the
// operator sees is a status line. "The button does nothing" and "the button
// errors invisibly" look identical from the far side of the screen.
//
// This lists what a person actually gets for each entry.
func TestMenuItemsWorkWithNoParameters(t *testing.T) {
	store := state.New(10)
	sim := &Sim{}
	Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)
	if _, err := store.Do(ctx, "project.open", "../../fixtures/fixture-fife-strict.json"); err != nil {
		t.Skip("no fixture:", err)
	}
	// Something selected, as there would be after a click on the map.
	_, _ = store.Do(ctx, "nodes.select", "Abernethy Repeater")

	// Exactly what the workbench's menus send today.
	// asks marks the entries that put a question on screen before the verb
	// runs, because the verb needs something a menu entry cannot carry. They
	// must fail without it - a verb that quietly invents a name or a pair of
	// nodes is worse than one that refuses.
	items := []struct {
		menu, label, verb string
		asks              bool
	}{
		{"File", "Open a saved network", "project.open", true},
		{"File", "Save this network", "project.save", true},
		{"File", "Save this run", "run.save", false},
		{"File", "Export the event log", "events.dump", false},
		{"Simulation", "One step", "sim.step", false},
		{"Simulation", "Back to the start", "sim.reset", false},
		{"Simulation", "Start firmware on every node", "firmware.start", false},
		{"Simulation", "Wipe every node's memory", "firmware.wipe", false},
		{"Simulation", "Originate a packet", "sim.inject", false},
		{"Simulation", "Capture the waterfall", "waterfall.capture", false},
		{"Simulation", "Capture to a pcapng file", "capture.file", false},
		{"Repeaters", "Coverage from the selection", "coverage.compute", false},
		{"Planning", "Routes between two selected nodes", "plan.routes", true},
		{"Help", "What this run assumes", "panel.Configuration", false},
	}

	var broken []string
	for _, it := range items {
		if strings.HasPrefix(it.verb, "panel.") {
			continue // the shell handles these itself
		}
		_, err := store.Do(ctx, it.verb, nil)
		if it.asks {
			if err == nil {
				t.Errorf("%s > %s ran with no parameter; the workbench asks for "+
					"one, so the verb accepting nothing means the answer was "+
					"discarded", it.menu, it.label)
			} else {
				t.Logf("asks %s > %s: %v", it.menu, it.label, err)
			}
			continue
		}
		if err != nil {
			broken = append(broken, it.menu+" > "+it.label+"  ("+it.verb+"): "+err.Error())
			continue
		}
		t.Logf("ok   %s > %s", it.menu, it.label)
	}
	if len(broken) > 0 {
		t.Errorf("menu entries that fail when picked, with nothing but a status line to say so:\n  %s",
			strings.Join(broken, "\n  "))
	}
}
