package ui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/MeshBench/meshbench/internal/firmware"

	"github.com/AllenDang/cimgui-go/imgui"
)

// consoleBuf collects what a node printed on its serial port.
//
// An io.Writer because that is what the bridge hands output to, and a mutex
// because the bridge writes from its own goroutine while the frame thread reads.
// Serial output arrives in whatever chunks the node happened to flush, not in
// lines, so this reassembles them.
type consoleBuf struct {
	// bridge is what this buffer is listening to, so a rebuilt engine gets a
	// fresh buffer attached to its new bridge rather than a stale one.
	bridge *firmware.Bridge
	// input is the half-typed command. Per node, not per panel: the same node's
	// console in the bottom tab and in its own window is the same terminal.
	input string
	// nowMs is the simulated clock at the moment output arrives, written by the
	// frame thread before each step and read by the bridge's goroutine.
	nowMs uint32

	mu      sync.Mutex
	lines   []string
	partial string
}

// maxConsoleLines is how much scrollback a node keeps.
//
// A repeater left running prints indefinitely, and an unbounded buffer turns a
// long scenario into a memory leak that shows up as the window slowing down.
const maxConsoleLines = 2000

func (c *consoleBuf) Write(p []byte) (int, error) {
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
		c.lines = append(c.lines, fmt.Sprintf("%8.3f  %s", float64(c.nowMs)/1000, line))
		c.partial = c.partial[i+1:]
	}
	if n := len(c.lines) - maxConsoleLines; n > 0 {
		c.lines = append(c.lines[:0], c.lines[n:]...)
	}
	return len(p), nil
}

// snapshot returns the scrollback plus whatever has arrived since the last
// newline, so a prompt with no line ending is still visible.
func (c *consoleBuf) snapshot() []string {
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
func (c *consoleBuf) mark() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.lines)
}

func (c *consoleBuf) linesSince(mark int) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if mark > len(c.lines) {
		return nil
	}
	out := make([]string, len(c.lines)-mark)
	copy(out, c.lines[mark:])
	return out
}

func (c *consoleBuf) echo(line string) {
	c.mu.Lock()
	c.lines = append(c.lines, fmt.Sprintf("%8.3f  > %s", float64(c.nowMs)/1000, line))
	c.mu.Unlock()
}

// drawConsole is the selected node's serial console.
//
// A real UART. The point of running real firmware is that its own command
// interface is how a node is configured, and a workbench that cannot reach it
// can build a mesh but not administer one. Everything below the input box came
// out of the firmware; nothing here composes a reply.
func (a *App) drawConsole() {
	from, _ := a.Link()
	if from < 0 {
		textDim("select a node")
		return
	}
	a.drawConsoleFor(a.Nodes[from].Name)
}

// drawConsoleFor is one node's console, wherever it is being shown — the
// bottom tab and any number of per-node windows share the same buffer, so the
// scrollback is the node's history rather than the panel's.
func (a *App) drawConsoleFor(name string) {
	if a.eng == nil {
		textDim("no simulation running - press \"run real firmware\" in the strip above")
		return
	}
	node, ok := a.eng.NodeByName(name)
	if !ok || node.Firmware == nil {
		textWrap("This node has no firmware attached, so there is no console to reach. " +
			"A console is the serial port of a running MeshCore build; without one there is " +
			"nothing on the other end, and a simulated prompt would be a claim about a program " +
			"that is not running.")
		imgui.Spacing()
		textDim("Press \"run real firmware\" in the strip above.")
		return
	}

	// consoleBufFor re-attaches when the engine was rebuilt: a new bridge that
	// nobody listens to is a console that looks alive and answers nothing.
	if node.Firmware.Bridge.Claimed() {
		textWrap("A companion client holds this node's serial port, so the console " +
			"is not reading it. Two protocols on one UART is neither - disconnect the " +
			"client on the Connect tab to take it back.")
		return
	}
	buf := a.consoleBufFor(name)
	if buf == nil {
		textDim("no console - this node has no firmware attached yet")
		return
	}

	buf.mu.Lock()
	buf.nowMs = a.eng.NowMs()
	buf.mu.Unlock()

	imgui.Text(name + " - serial console")
	imgui.SameLine()
	textDim(fmt.Sprintf("%s, t = %.2f s", node.Firmware.Backend.Kind(),
		float64(a.eng.NowMs())/1000))

	// The log first, so the input box stays at the bottom where a terminal puts
	// it. Reserving a row's height keeps it from being scrolled away.
	h := -imgui.FrameHeightWithSpacing()
	if imgui.BeginChildStrV("##consolelog"+name, imgui.NewVec2(0, h), 0,
		imgui.WindowFlagsHorizontalScrollbar) {
		lines := buf.snapshot()
		if len(lines) == 0 {
			textDim("(nothing printed yet)")
		}
		for _, line := range lines {
			imgui.TextUnformatted(line)
		}
		// Follow the tail only when already there, or reading scrollback becomes
		// impossible on a node that prints while you are looking at it.
		if imgui.ScrollY() >= imgui.ScrollMaxY() {
			imgui.SetScrollHereYV(1)
		}
	}
	imgui.EndChild()

	// What the radio heard, against what the firmware acted on.
	//
	// The spec's key example: a node whose firmware logged nothing while the RF
	// layer still reports a frame arrived and failed CRC. No firmware
	// instrumentation could show that gap — only something watching both sides
	// can, which is the whole argument for simulating rather than logging.
	var offered, decoded, sawIt int
	for _, r := range a.eng.Ledger.Rows() {
		if r.ToNode != name {
			continue
		}
		if r.Offered {
			offered++
		}
		if r.Demod && r.CRCOK {
			decoded++
		}
		if r.FirmwareSaw {
			sawIt++
		}
	}
	if offered > 0 {
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.6, 0.64, 0.72, 1))
		textWrap(fmt.Sprintf("RF layer: %d frames arrived, %d decoded, %d reached the "+
			"firmware. The difference is what the radio heard and the stack never saw.",
			offered, decoded, sawIt))
		imgui.PopStyleColor()
	}

	imgui.SetNextItemWidth(-60)
	entered := imgui.InputTextWithHint("##cmd"+name, "a command this firmware understands",
		&buf.input, imgui.InputTextFlagsEnterReturnsTrue, nil)
	imgui.SameLine()
	if (imgui.Button("send##"+name) || entered) && buf.input != "" {
		line := buf.input
		buf.echo(line)
		// CRLF: what a terminal sends, and what MeshCore's CLI is written to
		// expect from one.
		if err := node.Firmware.Bridge.Type([]byte(line + "\r\n")); err != nil {
			a.status = err.Error()
		}
		// Simulated time has to move for the firmware to read what was typed.
		// The node only runs inside a tick, so on a paused simulation a command
		// sits in its serial buffer unread — which looks exactly like a CLI that
		// ignores you. Half a second is enough for a reply and small enough not
		// to disturb a scenario someone is stepping through.
		a.stepEngine(50)
		buf.input = ""
		imgui.SetKeyboardFocusHereV(-1)
	}
}
