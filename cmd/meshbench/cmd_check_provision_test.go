package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/mesh/proto"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A scheduled line goes to the console the node actually has.
//
// fixture-fife-strict failed its own assertion for months on this: its three
// `public hello` sends were aimed at companions and written as console text.
// A companion has no console, so the bytes were read as somebody typing at a
// device that answers nothing - the run reported six sends, delivered nothing
// at all, and looked from the outside like a mesh that could not reach itself.
func TestAScheduledLineGoesToTheConsoleTheNodeHas(t *testing.T) {
	typed, err := scheduledLine(scenario.SimpleRepeater, "advert", 20000)
	if err != nil {
		t.Fatalf("a repeater's own CLI line was refused: %v", err)
	}
	if string(typed) != "advert\r\n" {
		t.Errorf("a repeater got %q, want the typed line", typed)
	}

	framed, err := scheduledLine(scenario.Companion, "public hello from Angus", 20000)
	if err != nil {
		t.Fatalf("a companion's scheduled message was refused: %v", err)
	}
	if strings.Contains(string(framed), "public hello") {
		t.Fatalf("a companion was sent console text: %q", framed)
	}
	if len(framed) < 4 || framed[0] != '<' {
		t.Fatalf("a companion was sent something that is not a frame: %q", framed)
	}
	if got := proto.Command(framed[3]); got != proto.CmdSendChannelTxtMsg {
		t.Errorf("the frame carries command %d, want a channel message", got)
	}
	// The text is in there, as a payload rather than as a line to type.
	if !bytes.Contains(framed, []byte("hello from Angus")) {
		t.Errorf("the message did not survive into the frame: %q", framed)
	}

	// A room server takes the same protocol, and used to take the same silence.
	if _, err := scheduledLine(scenario.RoomServer, "advert", 0); err != nil {
		t.Errorf("a room server's advert was refused: %v", err)
	}
}

// The same line at two moments of simulated time is stamped with each, so two
// runs of one seed produce the same bytes and a comparison means something.
func TestAScheduledMessageIsStampedInSimulatedTime(t *testing.T) {
	early, err := scheduledLine(scenario.Companion, "public hello", 20000)
	if err != nil {
		t.Fatalf("scheduledLine: %v", err)
	}
	late, err := scheduledLine(scenario.Companion, "public hello", 80000)
	if err != nil {
		t.Fatalf("scheduledLine: %v", err)
	}
	if bytes.Equal(early, late) {
		t.Error("two sends a minute apart carry the same timestamp")
	}
	again, err := scheduledLine(scenario.Companion, "public hello", 20000)
	if err != nil {
		t.Fatalf("scheduledLine: %v", err)
	}
	if !bytes.Equal(early, again) {
		t.Error("the same send at the same moment produced different bytes")
	}
}
