package session

import (
	"context"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// cliSim is a session with one companion, ready to take a command line.
func cliSim(t *testing.T) (*state.Store, *Sim, context.Context, func()) {
	t.Helper()
	st := state.New(10)
	s := &Sim{gpuAsked: true}
	Register(st, s)
	ctx, cancel := context.WithCancel(context.Background())
	go st.Run(ctx)
	nodes := []scenario.Node{repeaterNode("Alpha")}
	nodes[0].Kind = "companion"
	s.build(nodes, 869.618)
	// A session already in place, so the command line does not try to claim a
	// UART that no firmware is listening on. What is under test is where an
	// answer goes, not how the port is opened.
	s.comps = map[string]*compSession{"Alpha": {node: "Alpha"}}
	return st, s, ctx, cancel
}

// A command that fails must say so in the console it was typed into.
//
// It used to return the error from the verb, which sent the explanation to
// ui.said - the status bar and the session log - while the console showed
// nothing at all. Not even the command was echoed, because the echo was
// written on the success path, after the point the failure returned from.
func TestAFailedCommandAnswersInTheConsole(t *testing.T) {
	st, _, ctx, cancel := cliSim(t)
	defer cancel()

	res, err := st.Do(ctx, "console.cli",
		map[string]any{"node": "Alpha", "command": "region default #sco"})
	if err != nil {
		t.Fatalf("a failing command must not fail the verb: %v", err)
	}
	m, _ := res.(map[string]any)
	if m["failed"] != true {
		t.Errorf("the reply does not mark the command as failed: %+v", m)
	}
	reply, _ := m["reply"].(string)
	if !strings.Contains(reply, "no command") {
		t.Errorf("reply = %q, want it to name the missing command", reply)
	}

	snap := st.Snapshot()
	if snap.ConsoleNode != "Alpha" {
		t.Fatalf("console is showing %q, want Alpha", snap.ConsoleNode)
	}
	joined := strings.Join(snap.Console, "\n")
	if !strings.Contains(joined, "> region default #sco") {
		t.Errorf("the command was not echoed into the console:\n%s", joined)
	}
	if !strings.Contains(joined, "no command") {
		t.Errorf("the error is not in the console - it went to the log instead:\n%s", joined)
	}
}

// A command the real meshcore-cli has, but this does not, says which it is -
// and says it in the console.
func TestAnUnimplementedCommandSaysWhichItIs(t *testing.T) {
	st, _, ctx, cancel := cliSim(t)
	defer cancel()

	if _, err := st.Do(ctx, "console.cli",
		map[string]any{"node": "Alpha", "command": "reboot"}); err != nil {
		t.Fatalf("console.cli: %v", err)
	}
	joined := strings.Join(st.Snapshot().Console, "\n")
	if !strings.Contains(joined, "meshcore-cli has") {
		t.Errorf("a real meshcore-cli command was not distinguished from a typo:\n%s", joined)
	}
}

// The echo comes before the answer, so a console reads as a transcript.
func TestTheEchoPrecedesTheAnswer(t *testing.T) {
	st, _, ctx, cancel := cliSim(t)
	defer cancel()

	if _, err := st.Do(ctx, "console.cli",
		map[string]any{"node": "Alpha", "command": "nonsense"}); err != nil {
		t.Fatalf("console.cli: %v", err)
	}
	lines := st.Snapshot().Console
	echo, answer := -1, -1
	for i, l := range lines {
		if strings.HasPrefix(l, "> nonsense") {
			echo = i
		}
		if strings.Contains(l, "no command") {
			answer = i
		}
	}
	if echo < 0 || answer < 0 {
		t.Fatalf("missing echo (%d) or answer (%d): %v", echo, answer, lines)
	}
	if echo > answer {
		t.Errorf("the answer came before the command that caused it: %v", lines)
	}
}

// A CLI line must not steal the port from an attached outside client.
//
// The command line connects on first use, and connecting claims the UART -
// which, over a served port, unplugged whatever real client was on the other
// end without a word at either side.
func TestTheCLIRefusesAServedPort(t *testing.T) {
	st, s, ctx, cancel := cliSim(t)
	defer cancel()

	delete(s.comps, "Alpha")
	s.served = map[string]*engine.CompanionLink{"Alpha": {Node: "Alpha", Kind: "tcp", Addr: "127.0.0.1:1"}}
	res, err := st.Do(ctx, "console.cli",
		map[string]any{"node": "Alpha", "command": "infos"})
	if err != nil {
		t.Fatalf("a refusal must answer in the console, not fail the verb: %v", err)
	}
	m, _ := res.(map[string]any)
	if m["failed"] != true {
		t.Fatal("a CLI command took a served port instead of being refused")
	}
	if _, connected := s.comps["Alpha"]; connected {
		t.Fatal("the refused command claimed the port anyway")
	}
	if !strings.Contains(strings.Join(st.Snapshot().Console, "\n"), "served") {
		t.Errorf("the refusal should say the port is served, in the console")
	}
}

// The box says "? for the list", so ? must print the list - in the console,
// connected or not. It used to return the help only in the verb result,
// which the GUI discards: the advertised command produced nothing at all,
// not even its own echo.
func TestHelpAnswersInTheConsole(t *testing.T) {
	st, s, ctx, cancel := cliSim(t)
	defer cancel()

	// The stronger case: nothing connected yet. Help is local knowledge and
	// must not need a session to show up.
	delete(s.comps, "Alpha")
	if _, err := st.Do(ctx, "console.cli",
		map[string]any{"node": "Alpha", "command": "?"}); err != nil {
		t.Fatalf("console.cli ?: %v", err)
	}
	snap := st.Snapshot()
	if snap.ConsoleNode != "Alpha" {
		t.Fatalf("console is showing %q, want Alpha", snap.ConsoleNode)
	}
	joined := strings.Join(snap.Console, "\n")
	if !strings.Contains(joined, "> ?") {
		t.Errorf("? was not echoed:\n%s", joined)
	}
	if !strings.Contains(joined, "infos") || !strings.Contains(joined, "sync_msgs") {
		t.Errorf("the command list did not reach the console:\n%s", joined)
	}
}

// A connect that fails on first use must say so in the console it was typed
// into. It used to be returned as a verb error, which went to the status bar
// and left the console with no echo and no explanation.
func TestAFailedConnectAnswersInTheConsole(t *testing.T) {
	st, s, ctx, cancel := cliSim(t)
	defer cancel()

	// No session and no firmware attached, so connect-on-first-use fails.
	delete(s.comps, "Alpha")
	res, err := st.Do(ctx, "console.cli",
		map[string]any{"node": "Alpha", "command": "infos"})
	if err != nil {
		t.Fatalf("a failing connect must not fail the verb: %v", err)
	}
	m, _ := res.(map[string]any)
	if m["failed"] != true {
		t.Errorf("the reply does not mark the command as failed: %+v", m)
	}
	joined := strings.Join(st.Snapshot().Console, "\n")
	if !strings.Contains(joined, "> infos") {
		t.Errorf("the command was not echoed:\n%s", joined)
	}
	if !strings.Contains(joined, "no firmware") {
		t.Errorf("the failure's reason is not in the console:\n%s", joined)
	}
}
