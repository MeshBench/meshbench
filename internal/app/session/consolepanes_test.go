package session

import (
	"fmt"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// Typing at a second node keeps the first one's pane, and a node typed at
// again has its pane replaced rather than a second one added.
func TestConsolePanesKeepEveryNode(t *testing.T) {
	var w state.World
	showConsole(&w, "Filter-A", []string{"a1"})
	showConsole(&w, "Stock", []string{"s1"})
	showConsole(&w, "Filter-A", []string{"a1", "a2"})
	if w.ConsoleNode != "Filter-A" || len(w.Console) != 2 {
		t.Errorf("the single console is %q %q", w.ConsoleNode, w.Console)
	}
	if len(w.Consoles) != 2 {
		t.Fatalf("%d panes, want 2: %+v", len(w.Consoles), w.Consoles)
	}
	for _, p := range w.Consoles {
		if p.Node == "Stock" && (len(p.Lines) != 1 || p.Lines[0] != "s1") {
			t.Errorf("Stock's pane was overwritten: %q", p.Lines)
		}
	}
}

// The list is bounded, oldest out first.
func TestConsolePanesAreBounded(t *testing.T) {
	var w state.World
	for i := 0; i < maxConsolePanes+3; i++ {
		putConsole(&w, fmt.Sprintf("n%d", i), nil)
	}
	if len(w.Consoles) != maxConsolePanes {
		t.Fatalf("%d panes, want %d", len(w.Consoles), maxConsolePanes)
	}
	if w.Consoles[0].Node != "n3" {
		t.Errorf("the oldest kept is %s, want n3", w.Consoles[0].Node)
	}
}
