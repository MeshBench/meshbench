package session

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware/console"
)

// A command accepted on a paused run says so under the firmware's reply, and
// only then: not while playing, and not for a held call replayed inside a step.
func TestThePausedNoteAppliesOnlyWhenPaused(t *testing.T) {
	s := &Sim{}
	if got := s.pausedFor(&state.World{}); got != pausedNote {
		t.Errorf("paused: %q", got)
	}
	if got := s.pausedFor(&state.World{Playing: true}); got != "" {
		t.Errorf("playing: %q", got)
	}
	s.replaying = true
	if got := s.pausedFor(&state.World{}); got != "" {
		t.Errorf("replayed from the queue: %q", got)
	}
}

// The note is ours, so it is bracketed and cannot be read as the node's.
func TestANoteIsNotTheFirmwaresOutput(t *testing.T) {
	var b console.Buf
	b.SetNow(1500)
	_, _ = b.Write([]byte("  -> OK - Advert sent\n"))
	b.Note(pausedNote)
	lines := b.Snapshot()
	if len(lines) != 2 || !strings.HasSuffix(lines[1], "("+pausedNote+")") ||
		!strings.HasPrefix(strings.TrimSpace(lines[1]), "1.500") {
		t.Fatalf("lines: %q", lines)
	}
}

// The command line says it too, under the answer, and gives it to a script.
func TestTheCommandLineSaysTheRunIsPaused(t *testing.T) {
	st, _, ctx, cancel := cliSim(t)
	defer cancel()
	saved := meshcliCommands
	t.Cleanup(func() { meshcliCommands = saved })
	meshcliCommands = append(append([]cliCommand(nil), saved...), cliCommand{"__ok", "", "", "",
		func(*Sim, string, []string) (string, error) { return "advert sent", nil }})

	res, err := st.Do(ctx, "console.cli", map[string]any{"node": "Alpha", "command": "__ok"})
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := res.(map[string]any); m["paused"] != pausedNote {
		t.Errorf("the answer does not say the run is paused: %v", res)
	}
	lines := st.Snapshot().Console
	if len(lines) < 2 || lines[len(lines)-2] != "advert sent" || lines[len(lines)-1] != "("+pausedNote+")" {
		t.Errorf("the note is not under the answer: %q", lines)
	}
}
