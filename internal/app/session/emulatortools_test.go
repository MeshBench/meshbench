package session

// White-box, because what is being pinned is which tools a scenario's boards
// ask for, and a Sim holding nodes is the cheapest honest way to say it.

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware/emulated/renode"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

func simWithBoards(boards ...string) *Sim {
	s := &Sim{}
	for i, b := range boards {
		s.nodes = append(s.nodes, scenario.Node{
			Name:     string(rune('A' + i)),
			Firmware: scenario.FirmwareRef{Board: b},
		})
	}
	return s
}

// Which emulator a board needs is its MCU's business, and the count is what
// makes a missing tool blocking rather than optional.
func TestAScenarioSaysWhichEmulatorToolsItNeeds(t *testing.T) {
	got := simWithBoards("RAK_4631", "Heltec_v3", "").EmulatorToolsNeeded()
	want := map[string]int{
		// Every emulated node needs the radio model; the emulator follows the
		// MCU, and the node with no board is not emulated at all.
		"virtual-sx1262": 2, "renode": 1, "qemu-system-xtensa": 1,
	}
	for name, n := range want {
		if got[name] != n {
			t.Errorf("%s is needed by %d nodes, want %d (%v)", name, got[name], n, got)
		}
	}
}

// Starting firmware says what is missing before the boot rather than after it,
// and names the page that can fetch it.
//
// The environment variable is pointed at a path that is not there, which is
// the one way to make the lookup fail identically on a machine that has the
// emulators and one that does not.
func TestStartingFirmwareSaysAMissingToolBeforeItBoots(t *testing.T) {
	t.Setenv(renode.EnvRenode, "/nonexistent/renode")

	w := &state.World{}
	sim := simWithBoards("RAK_4631")
	sim.sayMissingEmulatorTools(w, sim.EmulatorToolsNeeded())
	said := strings.Join(w.Log, "\n")
	if !strings.Contains(said, "renode") || !strings.Contains(said, "Setup") {
		t.Errorf("an nRF52 node with no Renode was told:\n%s", said)
	}

	// And nothing at all where no node needs an emulator, because a warning
	// about a tool nothing here uses is one people learn to scroll past.
	quiet := &state.World{}
	none := simWithBoards("")
	none.sayMissingEmulatorTools(quiet, none.EmulatorToolsNeeded())
	if len(quiet.Log) != 0 {
		t.Errorf("a scenario with no emulated node was warned anyway: %v", quiet.Log)
	}
}
