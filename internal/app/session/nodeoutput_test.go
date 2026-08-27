package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
)

// outputStore is a store with node.output registered and the given nodes in
// place, which is all this verb reads of the world.
func outputStore(t *testing.T, nodes ...state.Node) (*state.Store, context.Context) {
	t.Helper()
	st := state.New(10)
	s := &Sim{}
	registerNodeOutput(st, s)
	st.Handle("test.nodes", func(w *state.World, p any) (any, error) {
		w.Nodes = p.([]state.Node)
		return nil, nil
	})
	st.Handle("test.world", func(w *state.World, p any) (any, error) {
		*(p.(**state.World)) = w
		return nil, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go st.Run(ctx)
	if _, err := st.Do(ctx, "test.nodes", nodes); err != nil {
		t.Fatal(err)
	}
	return st, ctx
}

// worldOf reaches the store's own world, to check what a verb published for
// the pane rather than what it answered the caller with. The two differ on
// purpose: the pane gets the tail, the caller gets a shorter one.
func worldOf(t *testing.T, st *state.Store, ctx context.Context) *state.World {
	t.Helper()
	var w *state.World
	if _, err := st.Do(ctx, "test.world", &w); err != nil {
		t.Fatal(err)
	}
	return w
}

// A log is not text. A companion build speaks its framed binary protocol on the
// same port it prints on, so the file holds both, and a pane handed the raw
// bytes draws replacement characters over the part somebody is looking for.
//
// This was measured on a real node's console.log, where the boot chain and the
// protocol frames are interleaved line by line.
func TestBinaryFramesStayReadableBesideTheText(t *testing.T) {
	in := []byte("rst:0x1 (POWERON)\r\n\x00\x91\x82boot ok\n\xff")
	got := printableLines(in)
	want := []string{
		"rst:0x1 (POWERON)",
		`\x00\x91\x82boot ok`,
		`\xff`,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lines %q, want %d %q", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d is %q, want %q", i, got[i], want[i])
		}
	}
}

// A terminal escape is an instruction to a terminal, not something the board
// said. Meshtastic colours every line it prints; rendered as hex, four escapes
// wrapped every word and the pane was unreadable.
func TestTerminalColoursDoNotDrownTheText(t *testing.T) {
	in := []byte("\x1b[32mINFO  \x1b[0m| Booted, wake cause 0\n")
	got := printableLines(in)
	if len(got) != 1 || got[0] != "INFO  | Booted, wake cause 0" {
		t.Errorf("got %q, want the line without its colours", got)
	}
}

// An empty source answers with an empty list, never with nothing at all. This
// crosses JSON, where a nil slice is null, and a caller indexing what the
// schema calls a list of lines gets a type error rather than no lines.
func TestAnEmptySourceStillAnswersWithAList(t *testing.T) {
	if got := printableLines(nil); got == nil {
		t.Error("an empty file gave a nil slice, which crosses the socket as null")
	}
}

// A file ending in a newline is not a file with a blank last line, and a bare
// carriage return ends a line on its own - ESP-IDF's bootloader uses both.
func TestLineEndingsDoNotInventLines(t *testing.T) {
	if got := printableLines([]byte("one\ntwo\n")); len(got) != 2 {
		t.Errorf("a trailing newline made %d lines: %q", len(got), got)
	}
	if got := printableLines([]byte("one\rtwo")); len(got) != 2 {
		t.Errorf("a bare carriage return made %d lines: %q", len(got), got)
	}
	if got := printableLines(nil); len(got) != 0 {
		t.Errorf("an empty file made %d lines", len(got))
	}
}

// The three sources are three different files, and for a node that is not
// emulated two of them do not exist as questions. Saying so is the point:
// an empty pane where there is no emulator reads as an emulator that has
// failed.
func TestEachSourceNamesItsOwnFileOrSaysWhyNot(t *testing.T) {
	dir := "/nodes/GB7XYZ"
	for _, c := range []struct {
		source   string
		emulated bool
		wantFile string
		wantNote string
	}{
		{"serial", true, "console.log", ""},
		{"serial", false, "stderr.log", ""},
		{"emulator", true, "emulator.log", ""},
		{"emulator", false, "", "no emulator"},
		{"radio", true, "radio.log", ""},
		{"radio", false, "", "keeps no log"},
	} {
		path, note, err := outputFile(dir, c.source, c.emulated)
		if err != nil {
			t.Fatalf("%s emulated=%v: %v", c.source, c.emulated, err)
		}
		if c.wantFile == "" {
			if path != "" {
				t.Errorf("%s emulated=%v gave a path %q where there is nothing to read",
					c.source, c.emulated, path)
			}
			if !strings.Contains(note, c.wantNote) {
				t.Errorf("%s emulated=%v said %q, which does not explain the empty pane",
					c.source, c.emulated, note)
			}
			continue
		}
		if filepath.Base(path) != c.wantFile {
			t.Errorf("%s emulated=%v reads %s, want %s",
				c.source, c.emulated, filepath.Base(path), c.wantFile)
		}
		if note != "" {
			t.Errorf("%s emulated=%v has a file and an excuse: %q", c.source, c.emulated, note)
		}
	}
	if _, _, err := outputFile(dir, "telepathy", true); err == nil {
		t.Error("an unknown source was accepted")
	}
}

// End to end: the verb finds the file the emulator writes, publishes it for the
// pane, and reports the whole size rather than the tail's.
func TestNodeOutputReadsWhatTheEmulatorWrote(t *testing.T) {
	root := t.TempDir()
	t.Setenv(firmware.EnvNodeFS, root)

	dir := firmware.NodeWorkDir("GB7XYZ")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for i := 0; i < outputTail+50; i++ {
		sb.WriteString("boot line\n")
	}
	if err := os.WriteFile(filepath.Join(dir, firmware.ConsoleLogName()),
		[]byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	st, ctx := outputStore(t, state.Node{Name: "GB7XYZ", Board: "Heltec_v3"},
		state.Node{Name: "host-1"})

	got, err := st.Do(ctx, "node.output", map[string]any{"node": "GB7XYZ", "lines": 5})
	if err != nil {
		t.Fatalf("node.output: %v", err)
	}
	m := got.(map[string]any)
	w := worldOf(t, st, ctx)
	if m["total"].(int) != outputTail+50 {
		t.Errorf("total is %v, want the whole file at %d", m["total"], outputTail+50)
	}
	if n := len(m["tail"].([]string)); n != 5 {
		t.Errorf("asked for 5 lines and got %d", n)
	}
	if len(w.Outputs) != 1 {
		t.Fatalf("published %d panes, want 1", len(w.Outputs))
	}
	pane := w.Outputs[0]
	if n := len(pane.Lines); n != outputTail {
		t.Errorf("the pane got %d lines, want the %d-line cap", n, outputTail)
	}
	if pane.Node != "GB7XYZ" || pane.Source != "serial" {
		t.Errorf("published %s/%s", pane.Node, pane.Source)
	}

	// A node with no board has no emulator, and the pane is told so rather
	// than shown an empty file.
	got, err = st.Do(ctx, "node.output", map[string]any{"node": "host-1", "source": "emulator"})
	if err != nil {
		t.Fatalf("node.output on a host build: %v", err)
	}
	if note, _ := got.(map[string]any)["note"].(string); note == "" {
		t.Error("a host build's emulator pane was left empty with no reason given")
	}

	// A node that is not there is refused rather than answered with silence.
	if _, err := st.Do(ctx, "node.output", map[string]any{"node": "nobody"}); err == nil {
		t.Error("node.output answered for a node that does not exist")
	}
}

// Before the first run there is no file. That is a fact about the node, not a
// failure, and the difference matters to somebody deciding whether the board
// is broken.
func TestAnUnrunNodeSaysItHasNotRun(t *testing.T) {
	t.Setenv(firmware.EnvNodeFS, t.TempDir())
	st, ctx := outputStore(t, state.Node{Name: "GB7XYZ", Board: "Heltec_v3"})

	got, err := st.Do(ctx, "node.output", "GB7XYZ")
	if err != nil {
		t.Fatalf("node.output: %v", err)
	}
	note, _ := got.(map[string]any)["note"].(string)
	if !strings.Contains(note, "not run") {
		t.Errorf("said %q of a node that has never started", note)
	}
}
