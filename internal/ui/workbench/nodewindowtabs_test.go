package workbench

import (
	"slices"
	"testing"

	"gioui.org/f32"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// Does a board get its Hardware tab whatever is running on it?
//
// It did not. The tab set was chosen by role, and the companion branch
// returned before the Hardware tab was ever added - so a T-Deck running a
// repeater had a Hardware tab and the same T-Deck running the companion the
// board was built for did not. The hardware belongs to the board; what is
// running on it is a separate question.
func TestEveryRoleWithABoardGetsTheHardwareTab(t *testing.T) {
	for _, kind := range []string{"companion", "simple-repeater", "advanced-repeater", "room-server"} {
		p := &nodeWindowPanel{}
		p.Kind = kind
		p.hasHardware = true
		if !slices.Contains(p.visibleTabs(), tabHardware) {
			t.Errorf("a %s on a board with a screen and buttons has no Hardware tab", kind)
		}
	}
}

// And a node with no hardware to show does not grow an empty one.
func TestNoHardwareMeansNoHardwareTab(t *testing.T) {
	for _, kind := range []string{"companion", "simple-repeater"} {
		p := &nodeWindowPanel{}
		p.Kind = kind
		if slices.Contains(p.visibleTabs(), tabHardware) {
			t.Errorf("a %s whose board declares nothing still offers a Hardware tab", kind)
		}
	}
}

// An observer is not a board: it runs no firmware and has nothing to draw, so
// it gets no Hardware tab even if something set the flag.
func TestAnObserverHasNoHardwareTab(t *testing.T) {
	p := &nodeWindowPanel{}
	p.Kind = "sdr-observer"
	p.hasHardware = true
	if slices.Contains(p.visibleTabs(), tabHardware) {
		t.Error("an SDR observer offers a Hardware tab; it is not a board")
	}
}

// The whole chain, not just the rule: a snapshot saying this node is a
// companion on a T-Deck has to end with a Hardware tab on screen.
//
// The rule above was right and the tab still did not appear, because what
// decides it is the board on the node's stats - and that was being read out
// of the session's node list by the engine's subscript, which is a different
// list in a different order.
func TestACompanionOnABoardDrawsItsHardwareTab(t *testing.T) {
	p := &nodeWindowPanel{}
	p.node, p.Kind = "Handheld", "companion"
	snap := &state.Snapshot{
		Nodes: []state.Node{{Name: "Handheld", Kind: "companion",
			Firmware: "wadamesh", Board: "LilyGo_TDeck"}},
		Stats: []state.NodeStat{{Name: "Handheld", Board: "LilyGo_TDeck",
			Backend: "emulated", Running: true}},
	}
	h := newPanelHarness(p.Draw, snap)
	h.frame()
	h.frame()
	if !slices.Contains(p.visibleTabs(), tabHardware) {
		t.Fatal("a companion whose stats name a LilyGo_TDeck has no Hardware " +
			"tab: the board is on the node and the tab is what shows it")
	}
}

// Does typing at a board with a keyboard reach it?
//
// The verb and the device were both working and nothing in the interface ever
// called them: there was no key handling on the tab at all, so a board with a
// keyboard could be looked at and not typed on.
func TestTypingAtABoardReachesItsKeyboard(t *testing.T) {
	var verbs []string
	var text string
	p := &nodeWindowPanel{}
	p.node, p.Kind = "Handheld", "companion"
	p.OnDo = func(verb string, params any) {
		verbs = append(verbs, verb)
		if m, ok := params.(map[string]any); ok {
			if s, ok := m["text"].(string); ok {
				text += s
			}
		}
	}
	snap := &state.Snapshot{
		Nodes: []state.Node{{Name: "Handheld", Kind: "companion",
			Board: "LilyGo_TDeck"}},
		Stats: []state.NodeStat{{Name: "Handheld", Board: "LilyGo_TDeck",
			Running: true}},
	}
	h := newPanelHarness(p.Draw, snap)
	p.tab = tabHardware
	h.frame()
	h.frame()
	// Click the drawn panel first, which is what puts the keyboard on the
	// board - then type, as somebody would.
	h.click(f32.Pt(120, 160))
	h.typeText("hi")
	if !slices.Contains(verbs, "board.key") {
		t.Fatal("typing at a board with a keyboard sent no board.key: the " +
			"tab draws the keyboard and never reads one")
	}
	if text != "hi" {
		t.Errorf("typed \"hi\" and the board was sent %q", text)
	}
}
