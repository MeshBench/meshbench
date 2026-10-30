package nodeview

import (
	"reflect"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// Two windows on two nodes each show their own node's console. With one shared
// console, the second said the first "has the console" and showed nothing.
func TestEachWindowShowsItsOwnConsole(t *testing.T) {
	s := &state.Snapshot{
		Console: []string{"> ver", "v1.18.0"}, ConsoleNode: "Stock",
		Consoles: []state.ConsolePane{
			{Node: "Filter-A", Lines: []string{"> filter stats", "dropped 3"}},
			{Node: "Stock", Lines: []string{"> ver", "v1.18.0"}},
		},
	}
	if got := consoleOf(s, "Filter-A"); !reflect.DeepEqual(got, []string{"> filter stats", "dropped 3"}) {
		t.Errorf("Filter-A's window shows %q", got)
	}
	if got := consoleOf(s, "Stock"); !reflect.DeepEqual(got, []string{"> ver", "v1.18.0"}) {
		t.Errorf("Stock's window shows %q", got)
	}
	if got := consoleOf(s, "Phone"); got != nil {
		t.Errorf("a node with no console shows %q", got)
	}

	// A snapshot from before the list still shows the one it carries.
	old := &state.Snapshot{Console: []string{"hello"}, ConsoleNode: "Stock"}
	if got := consoleOf(old, "Stock"); !reflect.DeepEqual(got, []string{"hello"}) {
		t.Errorf("the single console is not shown: %q", got)
	}
}
