package nodeview

import (
	"slices"
	"testing"

	"github.com/MeshBench/meshbench/pkg/client-go/meshbench"
)

// The clients carry their own copy of the tab names, because tools/clientgen
// cannot import this package - a code generator that pulled in the toolkit
// would be one nobody could run on a headless machine.
//
// So the copy is checked here instead, where the real list lives. Without this
// a tab renamed in the window would leave both clients offering a name
// node.window refuses, and the refusal would name the right tabs while the
// constant that produced it stayed wrong.
func TestTheClientsKnowTheSameTabs(t *testing.T) {
	mine := TabNames()
	theirs := make([]string, 0, len(meshbench.Tabs))
	for _, tab := range meshbench.Tabs {
		theirs = append(theirs, string(tab))
	}
	if !slices.Equal(mine, theirs) {
		t.Errorf("the window has %v and the clients have %v;\n"+
			"update tabs in tools/clientgen and run go run ./tools/clientgen",
			mine, theirs)
	}
	// And every one of them opens, which a list matching a list does not prove.
	for _, name := range theirs {
		if _, ok := TabByName(name); !ok {
			t.Errorf("the clients offer %q and the window will not open it", name)
		}
	}
}
