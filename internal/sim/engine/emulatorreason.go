// What the emulator said about stopping, for a node the run has dropped.
//
// Its own file because it is a different question from ticking a node: the
// tick loop knows a node has gone, and this knows where the reason is written
// down.
package engine

import (
	"strings"

	"github.com/MeshBench/meshbench/internal/firmware"
)

// says is a backend that keeps a log of the emulator's own complaints, as an
// emulated one does. Asserted rather than imported, the way the board probe
// asserts it: the native backend has no such log and this layer has no
// business knowing which backend is which.
type says interface {
	EmulatorLog() ([]byte, error)
}

// reasonWidth caps how much of that log travels with the failure. It goes into
// a line the operator reads in the events panel, and an emulator that filled
// its log with unimplemented-register warnings would otherwise put all of it
// there.
const reasonWidth = 160

// emulatorReason is the emulator's own last word, as a clause to append to the
// reason a node was dropped, or empty where there is nothing to add.
//
// "Stopped answering" is true of a board whose emulator aborted at start-up
// and true of a board whose firmware hung, and the two want opposite things
// done about them. The answer was already on disk both times and nothing
// pointed at it: an emulator that refused a machine property - "Attempt to set
// property 'apb_freq' ... after it was realized" - took every ESP32 board on
// every platform down, and read from the events panel as a mesh of boards that
// had each independently gone quiet.
func emulatorReason(fw *firmware.Node) string {
	if fw == nil {
		return ""
	}
	logged, ok := fw.Backend.(says)
	if !ok {
		return ""
	}
	out, err := logged.EmulatorLog()
	if err != nil {
		return ""
	}
	last := lastLine(string(out))
	if last == "" {
		return ""
	}
	if len(last) > reasonWidth {
		last = last[:reasonWidth] + "..."
	}
	return ". The emulator said: " + last
}

// lastLine is the final thing written to a log that is not blank.
//
// The last line rather than the first: QEMU's abort names the assertion, the
// file and the line before it names the property, and it is the property that
// says what is wrong.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}
