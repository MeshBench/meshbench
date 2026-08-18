// A node's console, kept where anything can read it.
//
// This lived in internal/ui, so the only way to see what a repeater printed
// was to open the imgui workbench. The buffer never needed a window: it is an
// io.Writer the firmware bridge writes into and a slice of lines somebody
// reads back.
package console

import (
	"fmt"
	"strings"
	"sync"

	"github.com/MeshBench/meshbench/internal/firmware"
)

type Buf struct {
	// Bridge is what this buffer is listening to, so a rebuilt engine gets a
	// fresh buffer attached to its new Bridge rather than a stale one.
	Bridge *firmware.Bridge
	// Input is the half-typed command. Per node, not per panel: the same node's
	// console in the bottom tab and in its own window is the same terminal.
	Input string
	// nowMs is the simulated clock at the moment output arrives, written
	// before each step and read by the bridge's goroutine as output lands.
	//
	// Behind the mutex, and reached through SetNow: it is written by whoever
	// steps the engine and read by the goroutine draining a serial port, and
	// an exported field was written by nobody at all - every line in the Gio
	// build's console was stamped 0.000.
	nowMs uint32

	mu      sync.Mutex
	lines   []string
	partial string
}

// MaxLines is how much scrollback a node keeps.
//
// A repeater left running prints indefinitely, and an unbounded buffer turns a
// long scenario into a memory leak that shows up as the window slowing down.
const MaxLines = 2000

func (c *Buf) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.partial += strings.ReplaceAll(string(p), "\r\n", "\n")
	for {
		i := strings.IndexByte(c.partial, '\n')
		if i < 0 {
			break
		}
		// Stamped with simulated time as it arrives. The value of four open
		// consoles is reading them at one instant, which is impossible with
		// four terminal windows and impossible here without this.
		line := strings.TrimRight(c.partial[:i], "\r")
		c.append(fmt.Sprintf("%8.3f  %s", float64(c.nowMs)/1000, line))
		c.partial = c.partial[i+1:]
	}
	return len(p), nil
}

// append adds a line and enforces MaxLines, under the caller's lock. Every
// path that grows c.lines goes through this - Write, Echo, Stamp, Section -
// so none of them can be the one that was forgotten when the bound was added.
func (c *Buf) append(line string) {
	c.lines = append(c.lines, line)
	if n := len(c.lines) - MaxLines; n > 0 {
		c.lines = append(c.lines[:0], c.lines[n:]...)
	}
}

// snapshot returns the scrollback plus whatever has arrived since the last
// newline, so a prompt with no line ending is still visible.
func (c *Buf) Snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.lines), len(c.lines)+1)
	copy(out, c.lines)
	if c.partial != "" {
		out = append(out, c.partial)
	}
	return out
}

// mark notes where the log currently ends, so linesSince can attribute what a
// command produced. Fleet dispatch depends on this pair.
func (c *Buf) Mark() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.lines)
}

func (c *Buf) LinesSince(mark int) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if mark > len(c.lines) {
		return nil
	}
	out := make([]string, len(c.lines)-mark)
	copy(out, c.lines[mark:])
	return out
}

func (c *Buf) Echo(line string) {
	c.mu.Lock()
	c.append(fmt.Sprintf("%8.3f  > %s", float64(c.nowMs)/1000, line))
	c.mu.Unlock()
}

// Stamp appends a line as though the firmware had printed it, timestamped the
// same way Write's own lines are. For synthetic lines that still belong in
// the transcript - a provisioning comment, say - but did not come from the
// serial port.
func (c *Buf) Stamp(line string) {
	c.mu.Lock()
	c.append(fmt.Sprintf("%8.3f  %s", float64(c.nowMs)/1000, line))
	c.mu.Unlock()
}

// Section appends a line with no timestamp at all - a divider, not an event.
// Provisioning uses it to mark where a run's transcript starts and how it
// concluded, visually distinct from anything the firmware actually said.
func (c *Buf) Section(line string) {
	c.mu.Lock()
	c.append(line)
	c.mu.Unlock()
}

// drawConsole is the selected node's serial console.
//
// A real UART. The point of running real firmware is that its own command
// interface is how a node is configured, and a workbench that cannot reach it
// can build a mesh but not administer one. Everything below the Input box came
// out of the firmware; nothing here composes a reply.

// SetNow tells the buffer what the clock reads, before the step that will
// produce the output it stamps.
func (c *Buf) SetNow(ms uint32) {
	c.mu.Lock()
	c.nowMs = ms
	c.mu.Unlock()
}
