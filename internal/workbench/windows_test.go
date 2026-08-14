package workbench

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/gui/shell"
)

// A question from a panel goes to the window the panel is in. This is the
// routing behind "clicking import in the Firmware window put the dialog in
// the main window" - the shared prompt lives in the main window, and a
// popped-out panel has its own.
func TestAPanelsQuestionsFollowItsWindow(t *testing.T) {
	w := newWindows()
	main := &shell.Prompt{}

	// Docked: questions go to the main window's prompt.
	if got := w.promptFor("Firmware", main); got != main {
		t.Fatal("a docked panel's questions should use the main prompt")
	}

	// Popped out: its own.
	own := &shell.Prompt{}
	w.mu.Lock()
	w.open["Firmware"] = true
	w.prompts["Firmware"] = own
	w.mu.Unlock()
	if got := w.promptFor("Firmware", main); got != own {
		t.Fatal("a popped-out panel's questions should use its window's prompt")
	}
	// Another panel is unaffected.
	if got := w.promptFor("Fleet", main); got != main {
		t.Fatal("another panel's questions should still use the main prompt")
	}

	// Closed again: back to the main window.
	w.mu.Lock()
	delete(w.open, "Firmware")
	delete(w.prompts, "Firmware")
	w.mu.Unlock()
	if got := w.promptFor("Firmware", main); got != main {
		t.Fatal("after the window closes, questions should return to the main prompt")
	}
}
