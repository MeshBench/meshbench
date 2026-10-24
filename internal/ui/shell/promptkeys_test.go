package shell

import (
	"testing"

	"gioui.org/io/key"
)

// A question being typed into keeps the keyboard.
//
// The menu shortcuts name no focus target, so they match whatever is focused,
// and Gio gives a key to the first handler in the frame that asks for it. The
// shortcuts were read at the top of the frame, ahead of the question's field,
// so a space typed into "Start a blank network where?" was taken as play.
func TestSpaceInAQuestionIsNotPlay(t *testing.T) {
	h := newHarness(t)
	h.sh.SetMenu("Simulation", []MenuItem{{Label: "Play", Action: "sim.start", Shortcut: "Space"}})
	h.sh.OnMenu = func(a string) { h.actions = append(h.actions, a) }
	h.sh.Ask.Open("Start a blank network where?", "a place", "Perth", func(string) {})
	h.frame()
	h.frame()

	h.r.Queue(key.Event{Name: key.NameSpace, State: key.Press},
		key.EditEvent{Range: key.Range{Start: 5, End: 5}, Text: " "})
	h.frame()
	for _, a := range h.actions {
		if a == "sim.start" {
			t.Fatal("a space typed into a question pressed play")
		}
	}
}

// Backspace and Delete edit the question's text.
func TestBackspaceEditsAQuestion(t *testing.T) {
	h := newHarness(t)
	h.sh.Ask.Open("Start a blank network where?", "a place", "Fifex", func(string) {})
	h.frame()
	h.sh.Ask.field.Editor.SetCaret(5, 5)
	h.frame()

	h.r.Queue(key.Event{Name: key.NameDeleteBackward, State: key.Press})
	h.frame()
	if got := h.sh.Ask.field.Editor.Text(); got != "Fife" {
		t.Fatalf("Backspace in a question left %q, want %q", got, "Fife")
	}
}

// The shortcut still works when nothing is being typed into.
func TestSpaceStillPlaysWithNoQuestion(t *testing.T) {
	h := newHarness(t)
	h.sh.SetMenu("Simulation", []MenuItem{{Label: "Play", Action: "sim.start", Shortcut: "Space"}})
	h.sh.OnMenu = func(a string) { h.actions = append(h.actions, a) }
	h.frame()

	h.r.Queue(key.Event{Name: key.NameSpace, State: key.Press})
	h.frame()
	if len(h.actions) != 1 || h.actions[0] != "sim.start" {
		t.Fatalf("Space with nothing focused fired %v, want sim.start", h.actions)
	}
}
