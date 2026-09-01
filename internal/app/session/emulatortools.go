// What an emulated board needs from this machine, asked before it is started
// rather than found out by starting it.
//
// A boot that fails for a missing emulator says so at the point it gives up,
// which is after the radio model is running and among the lines of a start
// that was already under way. Everything needed to know it in advance is here:
// which boards are involved, which tool each of their MCUs needs, and whether
// the lookup a boot would do can find it.
package session

import (
	"fmt"
	"sort"

	"github.com/MeshBench/meshbench/internal/app/resource"
	"github.com/MeshBench/meshbench/internal/app/state"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/firmware/emulated"
)

// EmulatorToolsNeeded counts the nodes here that cannot boot without each
// emulator tool, keyed by the tool's name.
//
// Exported because the resource page asks the same question to decide whether
// a missing tool is optional or blocking, and one loop answering it twice is
// two loops that come to disagree. Which tool a board needs is the catalogue's
// own MCU field rather than a list of board names kept anywhere.
func (s *Sim) EmulatorToolsNeeded() map[string]int {
	needed := map[string]int{}
	for _, n := range s.Nodes() {
		if !n.Firmware.Emulated() {
			continue
		}
		for _, name := range boardTools(n.Firmware.Board) {
			needed[name]++
		}
	}
	return needed
}

// boardTools is what one board's MCU has to have, and nothing where the board
// is unknown to this build.
func boardTools(board string) []string {
	b, err := hw.BoardByName(board)
	if err != nil {
		return nil
	}
	return resource.ToolsFor(b.MCU)
}

// boardToolCount is the same shape for one board about to be pinned, which is
// the other way an emulated node comes to be started.
func boardToolCount(board string) map[string]int {
	needed := map[string]int{}
	for _, name := range boardTools(board) {
		needed[name] = 1
	}
	return needed
}

// sayMissingEmulatorTools reports the tools a start is about to need and this
// machine has not got, and opens the page that can fetch them.
//
// Setup rather than a control of its own: that page already carries a Fetch on
// every toolchain row, with the size it will spend and the terms it arrives
// under, and a second button here would be a second place for the licence
// question to be got wrong. Opening it puts the row in front of somebody
// instead of leaving them to go looking for it.
func (s *Sim) sayMissingEmulatorTools(w *state.World, needed map[string]int) {
	var missing []string
	for name := range needed {
		if _, err := emulated.FindTool(name); err != nil {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return
	}
	sort.Strings(missing)
	for _, name := range missing {
		w.Say(fmt.Sprintf("%s is not on this machine, and %s. Help > Setup "+
			"has the download and what it will cost; resource.fetch does the "+
			"same from a script", name, nodesBlocked(needed[name])))
	}
	if s.ui != nil {
		// Best effort: a session with no window says the lines above and
		// nothing else, which is all a script wanted anyway.
		_ = s.ui.OpenPanel("Setup", "")
	}
}

// nodesBlocked says how much of this is stuck, in the singular where it is one
// node, because "1 node(s)" reads as a message nobody finished writing.
func nodesBlocked(n int) string {
	if n == 1 {
		return "a node here cannot boot without it"
	}
	return fmt.Sprintf("%d nodes here cannot boot without it", n)
}
