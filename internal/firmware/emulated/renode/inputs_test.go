package renode

import (
	"strings"
	"testing"
)

// A board with no inputs gets no input channel.
//
// Not tidiness: the model dials the port it is given and retries until the
// machine goes away, so one pointed at a port nobody opened would spend the
// run failing to connect and logging about it.
func TestABoardWithNoInputsGetsNoChannel(t *testing.T) {
	if got := Inputs(0, false); got != "" {
		t.Errorf("a board with no inputs was given a channel:\n%s", got)
	}
	if got := Inputs(0, true); got != "" {
		t.Errorf("a meter with no channel to arrive on was declared:\n%s", got)
	}
}

// The channel names both ports and, where the board has one, its converter.
func TestTheInputChannelNamesWhatItDrives(t *testing.T) {
	got := Inputs(41235, true)
	for _, want := range []string{
		"Miscellaneous.MeshBenchInputs @ sysbus",
		"port: 41235",
		"gpio0: gpio0",
		"gpio1: gpio1",
		"meter: saadc",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the channel does not say %q:\n%s", want, got)
		}
	}
	// A board with no cell to read has no meter to point at, and naming one
	// that is not in the platform is a description Renode refuses to load -
	// which takes the whole node down rather than one row of a table.
	if bare := Inputs(41235, false); strings.Contains(bare, "meter:") {
		t.Errorf("a board with no converter was pointed at one:\n%s", bare)
	}
}
