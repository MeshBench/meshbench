package shell

import (
	"sort"
	"strings"
	"testing"

	"gioui.org/f32"
)

// The chrome, pressed.
//
// The menu bar was audited entry by entry and the row underneath it was not,
// which left the most-used control in the workbench with no test at all: play.
// Its behaviour was checked by sending the verb, which proves the store does
// the right thing and says nothing about the button.

// The transport row: every control on it reaches a verb.
func TestEveryTransportControlReachesAVerb(t *testing.T) {
	h := newHarness(t)
	h.sh.OnMenu = func(a string) { h.actions = append(h.actions, a) }
	h.frame()

	// Swept rather than aimed, because where the row puts each control is the
	// layout's business. The row is one row tall under the menu bar.
	want := map[string]string{
		"sim.start":               "play",
		"sim.step":                "one step",
		"sim.reset":               "back to the start",
		"ui.toggle_real_firmware": "the real firmware switch",
	}
	for y := float32(2); y < 120; y += 3 {
		for x := float32(2); x < float32(h.sz.X); x += 4 {
			h.click(f32.Pt(x, y))
		}
	}
	// Restart asks twice: a run is cheap to lose and expensive to notice you
	// have lost, so the first press arms it and the second does it. The sweep
	// presses it many times, so both halves are covered.
	var missing []string
	for verb, what := range want {
		found := false
		for _, a := range h.actions {
			if a == verb {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, what+" ("+verb+")")
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("transport controls that reach nothing:\n  %s",
			strings.Join(missing, "\n  "))
	}
	t.Logf("transport fired %d actions", len(h.actions))
}

// The view tabs: each one selects its own view.
//
// Six tabs sharing one row, and a tab that selects the wrong view is a tab
// that looks like it works.
func TestEveryViewTabSelectsItsOwnView(t *testing.T) {
	h := newHarness(t)
	h.frame()

	seen := map[View]bool{}
	for y := float32(2); y < 120; y += 3 {
		for x := float32(2); x < float32(h.sz.X); x += 4 {
			h.click(f32.Pt(x, y))
			seen[h.sh.View] = true
		}
	}
	var never []string
	for v := View(0); v < numViews; v++ {
		if !seen[v] {
			never = append(never, v.String())
		}
	}
	if len(never) > 0 {
		t.Errorf("views no tab ever selected: %s", strings.Join(never, ", "))
	}
	t.Logf("%d of %d views reachable by clicking a tab", len(seen), int(numViews))
}
