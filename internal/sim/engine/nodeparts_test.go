package engine

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/firmware"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/firmware/emulated"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A board's parts reach the guest whichever emulator is running it.
//
// This is the regression that hid for a year: the Renode branch built its node
// and returned before any of the part wiring ran, so a declared button opened
// no channel, a declared card was never made and a declared display had nowhere
// to send a frame. Nothing failed - a press was accepted and moved no pin,
// which reads exactly like firmware that ignores the button.
//
// Named for what a person sees rather than for the function: the window draws
// both machines from one declaration, so if one of them silently wires less
// than the other, the window is lying about one of them.
func TestABoardsPartsAreWiredWhicheverEmulatorRunsIt(t *testing.T) {
	board := hw.Board{
		Name: "Invented", MCU: "nRF52840",
		Hardware: &hw.Panel{Parts: []hw.Part{
			{Kind: hw.Button, Name: "user", Pin: 42, ActiveLow: true},
			{Kind: hw.Meter, Name: "battery", Pin: 4, FullScaleMV: 14696},
		}},
	}
	spec := scenario.Node{Name: "wired-both-ways"}
	for _, emu := range []emulated.Emulator{emulated.Renode, emulated.QEMU} {
		node := &emulated.EmulatedNode{Emulator: emu, NodeName: spec.Name,
			Dir: t.TempDir()}
		got, err := withParts(node, board, spec, firmware.BuildSettings{})
		if err != nil {
			t.Fatalf("%v: %v", emu, err)
		}
		if got.Buttons == nil {
			t.Errorf("%v: the board declares a button and nothing listens for a "+
				"press, so pressing it can only move a pin nobody drives", emu)
		}
		defer func() { _ = got.Buttons.Close() }()
		if !got.HasMeter {
			t.Errorf("%v: the board declares a meter on AIN2 and the node says "+
				"it has none", emu)
		}
		// The channel each emulator's own model reaches it by differs, and both
		// have to be one or the other rather than neither.
		if got.ButtonPath == "" && got.ButtonPort == 0 {
			t.Errorf("%v: the input channel has neither a socket nor a port, so "+
				"nothing inside the machine can find it", emu)
		}
	}
}

// Renode cannot dial a socket file, and QEMU is given one.
//
// Two ways of saying the same thing, and picking the wrong one is silent: the
// device retries a port nobody opened for the life of the run.
func TestEachEmulatorGetsTheInputChannelItCanReach(t *testing.T) {
	board := hw.Board{Name: "Invented", MCU: "nRF52840",
		Renode: &hw.RenodeWiring{Platform: "x"},
		Hardware: &hw.Panel{Parts: []hw.Part{
			{Kind: hw.Button, Name: "user", Pin: 42},
		}},
	}
	node := &emulated.EmulatedNode{Emulator: emulated.Renode, Dir: t.TempDir()}
	got, err := withParts(node, board, scenario.Node{Name: "renode"},
		firmware.BuildSettings{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = got.Buttons.Close() }()
	if got.ButtonPort == 0 {
		t.Error("a Renode board's inputs are on no port, and it has no way to " +
			"dial the socket file the other machine uses")
	}

	board.Renode = nil
	board.QEMU = &hw.QEMUWiring{Machine: "x"}
	qnode := &emulated.EmulatedNode{Emulator: emulated.QEMU, Dir: t.TempDir()}
	qgot, err := withParts(qnode, board, scenario.Node{Name: "qemu"},
		firmware.BuildSettings{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = qgot.Buttons.Close() }()
	if qgot.ButtonPath == "" {
		t.Error("a QEMU board's inputs have no socket file")
	}
}
