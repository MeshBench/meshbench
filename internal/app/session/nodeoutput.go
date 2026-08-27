// What a node has actually said, from each of the three things that can say
// something about it.
//
// The console is a conversation: what somebody typed and what came back. This
// is not that. When a board black-screens there is no conversation to have, and
// the question is which of three voices went quiet - the board's own serial
// port, the emulator running it, or the radio model beside it. They were three
// files nobody could reach without a terminal, which is exactly the position
// somebody is in when the interface says the board is running and the board is
// doing nothing.
package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/control"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
	emu "github.com/MeshBench/meshbench/internal/firmware/emulated"
)

// outputTail is how many lines are published at once.
//
// A board that has been running an hour has more than anybody reads, and the
// whole file crosses the snapshot every frame. The count of what was left out
// travels with it, because a tail presented as the file is a pane that says a
// board stopped talking when it only stopped being shown.
const outputTail = 2000

// OutputSources are the voices, in the order somebody asks for them.
var OutputSources = []string{"serial", "rom", "emulator", "radio"}

// outputFile is which file a source is, and why it might be empty.
//
// The native backend has no emulator and no radio model of its own - it is the
// radio model - so those sources name what is missing rather than showing an
// empty pane, which reads as a component that has failed.
func outputFile(dir, source string, emulated bool) (path, note string, err error) {
	switch source {
	case "serial":
		if emulated {
			return filepath.Join(dir, emu.ConsoleLogName()), "", nil
		}
		// A native node prints to standard error; it has no serial port to
		// have. Same question, different wire.
		return filepath.Join(dir, "stderr.log"), "", nil
	case "rom":
		if !emulated {
			return "", "a build for this machine has no boot chain to read: it is " +
				"started by this process rather than by a ROM", nil
		}
		// Only a board whose application talks over USB has a separate one.
		// On every other board the ROM prints to the same port the firmware
		// does, and the serial pane already has it from the first byte.
		p := filepath.Join(dir, emu.ROMLogName())
		if !fileExists(p) {
			return "", "this board's console is UART0, so the ROM's own output is " +
				"at the top of the serial pane rather than in one of its own", nil
		}
		return p, "", nil
	case "emulator":
		if !emulated {
			return "", "this node runs on this machine rather than under an " +
				"emulator, so there is no emulator to have said anything", nil
		}
		return filepath.Join(dir, emu.EmulatorLogName()), "", nil
	case "radio":
		if !emulated {
			return "", "a native node is linked against the radio model rather " +
				"than talking to one, so the model keeps no log of its own", nil
		}
		return filepath.Join(dir, emu.RadioLogName()), "", nil
	}
	return "", "", fmt.Errorf("node.output: %q is not a source; try %s",
		source, strings.Join(OutputSources, ", "))
}

func registerNodeOutput(st *state.Store, s *Sim) {
	st.HandleSpec("node.output", state.Spec{
		What: "read what a node's serial port, emulator or radio model has printed",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true, Primary: true,
				What: "the node to read"},
			{Name: "source", Type: state.ParamString,
				What: "serial, emulator or radio; serial by default"},
			{Name: "lines", Type: state.ParamNumber,
				What: "how many lines of the tail to answer with, up to 2000"},
		},
		Returns: []string{"node", "source", "lines", "total", "path", "tail", "note", "tracing"},
	}, func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "node")
		if name == "" {
			return nil, badParams("node.output needs a node")
		}
		source, _ := namedField(p, "source")
		if source == "" {
			source = "serial"
		}
		emulated, err := s.nodeIsEmulated(w, name)
		if err != nil {
			return nil, err
		}
		path, note, err := outputFile(firmware.NodeWorkDir(name), source, emulated)
		if err != nil {
			return nil, err
		}

		lines := []string{}
		total := 0
		if path != "" {
			b, rerr := os.ReadFile(path)
			switch {
			case rerr != nil && os.IsNotExist(rerr):
				// Before the first run there is no file, and that is a fact
				// about the node rather than a failure to report.
				note = "nothing yet: this node has not run since the workbench started"
			case rerr != nil:
				return nil, rerr
			default:
				lines = printableLines(b)
				total = len(lines)
				if len(lines) > outputTail {
					lines = lines[len(lines)-outputTail:]
				}
			}
		}

		putOutput(w, state.OutputPane{
			Node: name, Source: source, Lines: lines, Total: total,
			Path: path, Note: note, Tracing: emu.QEMUTracing(),
		})

		// The answer is a shorter tail than the pane gets: a script asking over
		// the control socket is usually asking a question about the last few
		// lines, and the whole two thousand is a megabyte down a pipe.
		// An empty list rather than nothing at all: this crosses JSON, where
		// a nil slice is null, and a caller indexing what the schema says is
		// a list of lines gets a type error instead of no lines.
		tail := lines
		want := 200
		if n, ok := numField(p, "lines"); ok && n > 0 {
			want = int(n)
		}
		if len(tail) > want {
			tail = tail[len(tail)-want:]
		}
		return map[string]any{
			"node": name, "source": source, "lines": len(lines), "total": total,
			"path": path, "tail": tail, "note": note,
			"tracing": emu.QEMUTracing(),
		}, nil
	})
}

// putOutput lands one pane's content in the world, replacing that pane's
// previous content and leaving every other pane alone.
//
// The list is keyed by node and source together. One slot for all of them was
// what it was: two windows on two nodes overwrote each other every tick, and
// switching source blanked the pane until the next tick landed. Nothing on
// disk was ever lost - what was lost was the one slot they shared.
func putOutput(w *state.World, pane state.OutputPane) {
	for i := range w.Outputs {
		if w.Outputs[i].Node == pane.Node && w.Outputs[i].Source == pane.Source {
			w.Outputs[i] = pane
			return
		}
	}
	if len(w.Outputs) >= maxOutputPanes {
		// A bound, because every pane is re-read on every tick and each one
		// costs a file read. Reached only by opening more output windows than
		// anybody has screen for; the oldest goes, and its pane asks again on
		// its next frame because it will not find itself here.
		w.Outputs = w.Outputs[1:]
	}
	w.Outputs = append(w.Outputs, pane)
}

// maxOutputPanes is how many node-and-source pairs are re-read per tick.
const maxOutputPanes = 12

// refreshOutput re-reads every file a pane is showing.
//
// Called from the tick rather than by the pane, because the emulator is still
// writing: a pane that reads once shows a board that stopped talking the moment
// somebody looked at it. Failures leave a pane as it was - a file that has gone
// is what the pane last saw plus no new lines, which is nearer the truth than
// an empty pane.
func (s *Sim) refreshOutput(w *state.World) {
	for i := range w.Outputs {
		s.refreshOnePane(w, &w.Outputs[i])
	}
}

func (s *Sim) refreshOnePane(w *state.World, pane *state.OutputPane) {
	emulated, err := s.nodeIsEmulated(w, pane.Node)
	if err != nil {
		return
	}
	path, note, err := outputFile(firmware.NodeWorkDir(pane.Node), pane.Source, emulated)
	if err != nil || path == "" {
		pane.Path, pane.Note = path, note
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := printableLines(b)
	pane.Total, pane.Path, pane.Note = len(lines), path, note
	if len(lines) > outputTail {
		lines = lines[len(lines)-outputTail:]
	}
	pane.Lines = lines
}

// nodeIsEmulated says which backend a node is running on, and refuses a node
// that is not there at all.
//
// Answered from the engine when the node is running and from the scenario when
// it is not, because the question is asked most often of a node that has just
// stopped - which is when its output is most worth reading.
func (s *Sim) nodeIsEmulated(w *state.World, name string) (bool, error) {
	if s.eng != nil {
		if n, ok := s.eng.NodeByName(name); ok && n.Firmware != nil {
			return n.Firmware.Backend.Kind() == "emulated", nil
		}
	}
	for i := range w.Nodes {
		if w.Nodes[i].Name == name {
			// A board is what makes a node emulated: a node with none runs a
			// build made for this machine.
			return w.Nodes[i].Board != "", nil
		}
	}
	return false, noSuchNode(name)
}

// fileExists answers whether a path is a readable file.
//
// A separate question from "did a read fail": whether a board keeps a separate
// boot log is a fact about the board, and reporting its absence as an error
// would make a normal configuration look like a fault.
func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// printableLines splits a log into lines a pane can draw.
//
// The bytes are not text. A companion build speaks its framed binary protocol
// on the same port it prints on, so a log holds both, and a pane handed the
// raw bytes draws replacement characters where the interesting part is. The
// frames are shown as their hex rather than dropped: what somebody is looking
// for is often that they are there at all.
func printableLines(b []byte) []string {
	out := []string{}
	var line strings.Builder
	flush := func() {
		out = append(out, line.String())
		line.Reset()
	}
	for i := 0; i < len(b); i++ {
		c := b[i]
		// A terminal escape is an instruction to a terminal, not something the
		// board said. Meshtastic colours every line it prints, and rendering
		// those bytes as hex put four escapes around every word - the pane was
		// legible only to somebody willing to read past them.
		if c == 0x1B && i+1 < len(b) && b[i+1] == '[' {
			j := i + 2
			for j < len(b) && (b[j] < 0x40 || b[j] > 0x7E) {
				j++
			}
			i = j
			continue
		}
		switch {
		case c == '\n':
			flush()
		case c == '\r':
			// A bare carriage return is a line ending too; one followed by a
			// newline is half of one.
			if i+1 < len(b) && b[i+1] == '\n' {
				continue
			}
			flush()
		case c == '\t' || (c >= 0x20 && c < 0x7F):
			line.WriteByte(c)
		default:
			fmt.Fprintf(&line, "\\x%02x", c)
		}
	}
	if line.Len() > 0 {
		flush()
	}
	// A file ending in a newline is not a file with a blank last line.
	if n := len(out); n > 0 && out[n-1] == "" {
		out = out[:n-1]
	}
	return out
}

func registerNodeOutputWindow(st *state.Store, s *Sim) {
	// node.output_window: one log, on its own, beside the board it came from.
	//
	// A tab is one pane, and what people do while a board is misbehaving is
	// watch its screen and two of its logs together - what the board printed
	// beside what the emulator said about running it.
	st.HandleSpec("node.output_window", state.Spec{
		What: "Open one node's one log in a window of its own, so a board's screen and several of its logs can be watched at once.",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true, Primary: true,
				What: "which node"},
			{Name: "source", Type: state.ParamString,
				What: "which log: " + strings.Join(OutputSources, ", ") + "; serial when absent"},
		},
		Returns: []string{"node", "source"},
	}, func(w *state.World, p any) (any, error) {
		if err := s.needUI(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "node")
		if name == "" {
			return nil, badParams("node.output_window needs a node")
		}
		if _, err := s.nodeIsEmulated(w, name); err != nil {
			return nil, err
		}
		source, _ := namedField(p, "source")
		if source == "" {
			source = "serial"
		}
		if !knownOutputSource(source) {
			return nil, badParams("no output called %q - there is %s",
				source, strings.Join(OutputSources, ", "))
		}
		if err := s.ui.OpenOutputWindow(name, source); err != nil {
			return nil, control.WithCode(control.BadParams, err)
		}
		return map[string]any{"node": name, "source": source}, nil
	})
}

// knownOutputSource reports whether this is one of the voices a node has.
func knownOutputSource(source string) bool {
	for _, s := range OutputSources {
		if s == source {
			return true
		}
	}
	return false
}
