package nodeview

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// The pane drew "nothing printed yet - start the node, or type a command" for
// a node that was running and had printed, because only one node's console is
// attached at a time and this one was not it. Telling a reader to start a node
// the header two lines above says is running is worse than saying nothing.
func TestTheConsoleSaysWhoHasIt(t *testing.T) {
	got := consoleEmptyState(&state.Snapshot{ConsoleNode: "Abernethy Repeater"}, "West Lomond")
	if !strings.Contains(got, "Abernethy Repeater") {
		t.Errorf("the empty state does not say which node has the console: %q", got)
	}
	if strings.Contains(got, "start the node") {
		t.Errorf("the empty state still tells the reader to start a node: %q", got)
	}

	// Its own console, genuinely empty: the original sentence is right.
	got = consoleEmptyState(&state.Snapshot{ConsoleNode: "West Lomond"}, "West Lomond")
	if !strings.Contains(got, "nothing printed yet") {
		t.Errorf("a node holding its own empty console says %q", got)
	}

	// Nothing attached anywhere: also the original sentence.
	got = consoleEmptyState(&state.Snapshot{}, "West Lomond")
	if !strings.Contains(got, "nothing printed yet") {
		t.Errorf("with no console attached at all the pane says %q", got)
	}
}
