package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MeshBench/meshbench/internal/mesh/firmware"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A build's settings reach the machine that runs it.
//
// They are read from beside the image rather than from the board, because the
// same hardware runs one image that needs the coprocessors up at reset and
// another that would be flattered by it.
func TestABuildsSettingsReachTheMachine(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("HOME", cache)
	t.Setenv(firmware.EnvNodeFS, t.TempDir())

	board := "LilyGo_TDeck"
	dir := filepath.Join(firmware.DefaultCacheDir(), "board", board)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A whole flash image, because the backend pads one before it runs.
	img := make([]byte, 0x9000)
	img[0], img[3] = 0xE9, 0x40
	img[0x8000], img[0x8001] = 0xAA, 0x50
	image := filepath.Join(dir, "companion_radio_usb@mesh-rs.bin")
	if err := os.WriteFile(image, img, 0o644); err != nil {
		t.Fatal(err)
	}
	spec := scenario.Node{
		Name: "Deck", Kind: scenario.Companion, Board: board,
		Firmware: scenario.FirmwareRef{Version: "mesh-rs", Board: board,
			Role: scenario.RoleCompanionRadioUSB},
	}

	// Off until asked for: the machine reports a register the way silicon does
	// not, and a firmware that genuinely mismanages it would be flattered.
	node, err := emulatedBackend(spec, true)
	if err != nil {
		t.Fatalf("emulatedBackend: %v", err)
	}
	if node.CoprocAtReset {
		t.Error("a build that asked for nothing got enabled coprocessors")
	}
	if declared, err := scenario.BoardByName(board); err != nil {
		t.Fatal(err)
	} else if node.SPI != declared.QEMU.SPI {
		t.Errorf("the node is on controller %d, want the board's %d",
			node.SPI, declared.QEMU.SPI)
	}

	if err := firmware.SaveBuildSettings(image,
		firmware.BuildSettings{CoprocAtReset: true}); err != nil {
		t.Fatal(err)
	}
	node, err = emulatedBackend(spec, true)
	if err != nil {
		t.Fatalf("emulatedBackend: %v", err)
	}
	if !node.CoprocAtReset {
		t.Error("the build asked for enabled coprocessors and did not get them")
	}
}

// A slot is not a fitted card: the board says the slot exists, the node says
// whether it is filled, and a firmware that keeps its settings on one fills it
// regardless.
func TestTheCardSlotIsTheNodesUntilTheFirmwareInsists(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("HOME", cache)
	t.Setenv(firmware.EnvNodeFS, t.TempDir())

	board := "LilyGo_TDeck"
	dir := filepath.Join(firmware.DefaultCacheDir(), "board", board)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	img := make([]byte, 0x9000)
	img[0], img[3] = 0xE9, 0x40
	img[0x8000], img[0x8001] = 0xAA, 0x50
	image := filepath.Join(dir, "companion_radio_usb@mesh-rs.bin")
	if err := os.WriteFile(image, img, 0o644); err != nil {
		t.Fatal(err)
	}
	spec := scenario.Node{
		Name: "Deck", Kind: scenario.Companion, Board: board,
		Firmware: scenario.FirmwareRef{Version: "mesh-rs", Board: board,
			Role: scenario.RoleCompanionRadioUSB},
	}

	// The board's own answer: a card in every slot it declares, which is what
	// happened before any of this existed.
	node, err := emulatedBackend(spec, true)
	if err != nil {
		t.Fatalf("emulatedBackend: %v", err)
	}
	if node.CardPath == "" {
		t.Fatal("a T-Deck came up with no card at all")
	}

	// Taken out, and it stays out.
	spec.Card = scenario.CardEmpty
	node, err = emulatedBackend(spec, true)
	if err != nil {
		t.Fatal(err)
	}
	if node.CardPath != "" {
		t.Errorf("the slot was emptied and the machine still got %s", node.CardPath)
	}

	// Unless the firmware will not boot without one, which it can say.
	if err := firmware.SaveBuildSettings(image,
		firmware.BuildSettings{CardRequired: true}); err != nil {
		t.Fatal(err)
	}
	node, err = emulatedBackend(spec, true)
	if err != nil {
		t.Fatal(err)
	}
	if node.CardPath == "" {
		t.Error("a build that needs storage was started without any")
	}

	// And a card the node was handed is the one it gets.
	mine := filepath.Join(t.TempDir(), "shared.img")
	spec.Card, spec.CardFile = scenario.CardFitted, mine
	node, err = emulatedBackend(spec, true)
	if err != nil {
		t.Fatal(err)
	}
	if node.CardPath != mine {
		t.Errorf("it was handed %s and got %s", mine, node.CardPath)
	}
}
