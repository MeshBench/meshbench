package boardview

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// A companion's console shows the decoded exchange, not the wire.
//
// Its serial carries the framed protocol, so a byte-at-a-time rendering is a
// wall of \x00\x05 with the answer buried in it - typing "ver" at one produced
// exactly that, with the board name and the firmware version legible inside the
// escapes. console.cli already keeps the decoded transcript; this draws that.
func TestACompanionShowsItsTranscriptRatherThanItsWire(t *testing.T) {
	s := &state.Snapshot{
		Nodes:       []state.Node{{Name: "citizen71", Kind: "companion"}},
		Console:     []string{"> ver", "v1.17.1-d929643"},
		ConsoleNode: "citizen71",
	}
	lines, isComp := companionTranscript(s, "citizen71")
	if !isComp {
		t.Fatal("a companion was not recognised, so its wire would be drawn")
	}
	if len(lines) != 2 || lines[1] != "v1.17.1-d929643" {
		t.Errorf("the transcript came back as %v", lines)
	}

	// One node's transcript is not another's.
	if got, _ := companionTranscript(s, "someone-else"); got != nil {
		t.Errorf("a different node was shown citizen71's conversation: %v", got)
	}

	// And a repeater keeps its serial, which really is text.
	r := &state.Snapshot{Nodes: []state.Node{{Name: "Rpt", Kind: "simple-repeater"}}}
	if _, isComp := companionTranscript(r, "Rpt"); isComp {
		t.Error("a repeater was treated as a companion, so its console - which " +
			"is text and readable - would be replaced by a transcript it has none of")
	}
}

// The wire is what shows until decoding is asked for.
//
// This window is about what the board actually did, so the bytes it actually
// sent are the default and the decode is an aid somebody turns on. A tick that
// started on would be this window quietly choosing to show something other than
// what happened.
func TestDecodingIsOffUntilItIsAskedFor(t *testing.T) {
	p := &Panel{Node: "citizen71"}
	if p.decode.Bool.Value {
		t.Error("decoding starts on, so the console shows an interpretation " +
			"before anybody asked for one")
	}
	// And the tick is only offered where there is something framed to decode.
	if p.framed {
		t.Error("the tick is offered before anything has said this node speaks " +
			"a framed protocol")
	}
}
