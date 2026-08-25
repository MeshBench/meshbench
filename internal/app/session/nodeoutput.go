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

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/mesh/firmware"
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
			return filepath.Join(dir, firmware.ConsoleLogName()), "", nil
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
		p := filepath.Join(dir, firmware.ROMLogName())
		if _, err := os.Stat(p); err != nil {
			return "", "this board's console is UART0, so the ROM's own output is " +
				"at the top of the serial pane rather than in one of its own", nil
		}
		return p, "", nil
	case "emulator":
		if !emulated {
			return "", "this node runs on this machine rather than under an " +
				"emulator, so there is no emulator to have said anything", nil
		}
		return filepath.Join(dir, firmware.EmulatorLogName()), "", nil
	case "radio":
		if !emulated {
			return "", "a native node is linked against the radio model rather " +
				"than talking to one, so the model keeps no log of its own", nil
		}
		return filepath.Join(dir, firmware.RadioLogName()), "", nil
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

		w.Output, w.OutputNode, w.OutputSource = lines, name, source
		w.OutputTotal, w.OutputPath, w.OutputNote = total, path, note

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
			"tracing": firmware.QEMUTracing(),
		}, nil
	})
}

// refreshOutput re-reads the file the output pane is showing.
//
// Called from the tick rather than by the pane, because the emulator is still
// writing: a pane that reads once shows a board that stopped talking the moment
// somebody looked at it. Failures are left in place - a file that has gone is
// what the pane last saw plus no new lines, which is nearer the truth than an
// empty pane.
func (s *Sim) refreshOutput(w *state.World) {
	emulated, err := s.nodeIsEmulated(w, w.OutputNode)
	if err != nil {
		return
	}
	path, note, err := outputFile(firmware.NodeWorkDir(w.OutputNode), w.OutputSource, emulated)
	if err != nil || path == "" {
		w.OutputPath, w.OutputNote = path, note
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := printableLines(b)
	w.OutputTotal, w.OutputPath, w.OutputNote = len(lines), path, note
	if len(lines) > outputTail {
		lines = lines[len(lines)-outputTail:]
	}
	w.Output = lines
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
