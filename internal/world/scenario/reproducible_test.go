package scenario_test

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/world/scenario"
)

func withFirmware(name, board string) scenario.Node {
	return scenario.Node{
		Name:     name,
		Firmware: scenario.FirmwareRef{Role: scenario.RoleSimpleRepeater, Board: board},
	}
}

// A mesh of native nodes keeps the guarantee, and says nothing.
//
// The empty answer is the load-bearing half. Everything downstream treats a
// sentence as a reason to warn, so a helper that hedged here - "probably
// reproducible" - would put a caveat on every run the simulator makes and
// teach people to scroll past the one that means something.
func TestANativeScenarioIsReproducible(t *testing.T) {
	nodes := []scenario.Node{
		withFirmware("alpha", ""), withFirmware("bravo", ""),
	}
	if why := scenario.NotReproducible(nodes); why != "" {
		t.Errorf("a native-only scenario reported %q; the seed decides all of it", why)
	}
	if got := scenario.EmulatedNodes(nodes); len(got) != 0 {
		t.Errorf("EmulatedNodes found %v where no node names a board", got)
	}
}

// One emulated node in a native mesh is the shape the docs recommend, and it
// is the shape that breaks the guarantee for the whole run.
func TestOneEmulatedNodeMakesTheWholeRunNotReproducible(t *testing.T) {
	nodes := []scenario.Node{
		withFirmware("alpha", ""),
		withFirmware("kelpie", "Heltec_v3"),
		withFirmware("bravo", ""),
	}
	why := scenario.NotReproducible(nodes)
	if why == "" {
		t.Fatal("a scenario carrying an emulated node reported itself reproducible")
	}
	// The name, because the first question anybody asks is which node, and an
	// answer that omits it sends them through the node list for something that
	// was already in hand.
	if !strings.Contains(why, "kelpie") {
		t.Errorf("the reason does not name the node responsible: %q", why)
	}
	// And the consequence, because "not reproducible" on its own reads as a
	// fault to be fixed rather than as a limit on what may be compared.
	if !strings.Contains(why, "compared") {
		t.Errorf("the reason does not say what follows from it: %q", why)
	}
	if got := scenario.EmulatedNodes(nodes); len(got) != 1 || got[0] != "kelpie" {
		t.Errorf("EmulatedNodes answered %v, want [kelpie]", got)
	}
}

// Several of them are named while they can still be read, and counted after.
func TestTheReasonNamesTheEmulatedNodesWhileItCan(t *testing.T) {
	for _, tc := range []struct {
		names []string
		want  string
	}{
		{[]string{"a", "b"}, "a and b run"},
		{[]string{"a", "b", "c"}, "a, b and c run"},
		{[]string{"a", "b", "c", "d"}, "a, b and 2 other nodes run"},
	} {
		why := scenario.NotReproducibleWith(tc.names)
		if !strings.HasPrefix(why, tc.want) {
			t.Errorf("%d emulated nodes read %q, want it to open with %q",
				len(tc.names), why, tc.want)
		}
	}
	if why := scenario.NotReproducibleWith(nil); why != "" {
		t.Errorf("no emulated nodes still produced %q", why)
	}
}
