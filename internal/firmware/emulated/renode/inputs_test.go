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

// The display goes on the controller the board drives it from.
//
// Not the radio's. Both Heltec boards here put their panel on the Arduino
// core's SPI1, which is the controller this already declares so that firmware
// does not block on one nothing answers - and it had been declared empty for a
// year with the reason written beside it.
func TestTheDisplayGoesOnTheSecondController(t *testing.T) {
	got := Panel(0x4002F000, 42505, 240, 135, 11, 12, "gpio0")
	for _, want := range []string{
		"panel: Video.MeshBenchPanel @ spi2",
		"width: 240",
		"height: 135",
		"csPin: 11",
		"dcPin: 12",
		"11 -> panel@11",
		"12 -> panel@12",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the description does not say %q:\n%s", want, got)
		}
	}
}

// Nothing where there is nothing to declare.
//
// A board with no panel, and a board whose radio is on the controller a panel
// would go on: neither can be described, and describing one anyway produces a
// platform Renode refuses to load - which takes the node down rather than one
// row of a table.
func TestNoDisplayIsDescribedWhereNoneCanBe(t *testing.T) {
	for _, c := range []struct {
		what string
		got  string
	}{
		{"a board with no panel", Panel(0x4002F000, 0, 0, 0, 0, 0, "gpio0")},
		{"a panel with no channel", Panel(0x4002F000, 0, 240, 135, 11, 12, "gpio0")},
		{"a radio on the panel's controller",
			Panel(stockSPIBase, 42505, 240, 135, 11, 12, "gpio0")},
	} {
		if c.got != "" {
			t.Errorf("%s was given a display:\n%s", c.what, c.got)
		}
	}
}
