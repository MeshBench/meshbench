package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MeshBench/meshbench/internal/mesh/firmware"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A build that drives the other SPI controller gets the other SPI controller,
// and its peripherals move with it.
//
// The pins are fixed in copper and the matrix routes whichever controller the
// firmware picks onto them, so two builds for one board can differ and both be
// right. Wired for the wrong one, the radio, the card and the screen all
// answer nothing - which reads as a board with nothing fitted, and is what
// sent one investigation looking at the radio for a day.
func TestABuildCanNameTheSPIControllerItDrives(t *testing.T) {
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
		Name: "Deck", Kind: scenario.Companion,
		Firmware: scenario.FirmwareRef{Version: "mesh-rs", Board: board,
			Role: scenario.RoleCompanionRadioUSB},
	}
	declared, err := scenario.BoardByName(board)
	if err != nil {
		t.Fatal(err)
	}

	// Nothing decided: the board's own answer.
	node, err := emulatedBackend(spec, true)
	if err != nil {
		t.Fatalf("emulatedBackend: %v", err)
	}
	if node.SPI != declared.QEMU.SPI {
		t.Fatalf("with nothing decided the node is on controller %d, want the "+
			"board's %d", node.SPI, declared.QEMU.SPI)
	}

	// And the build's answer where it has one.
	other := 2
	if declared.QEMU.SPI == 2 {
		other = 3
	}
	if err := firmware.SaveBuildSettings(image,
		firmware.BuildSettings{SPIController: other}); err != nil {
		t.Fatal(err)
	}
	node, err = emulatedBackend(spec, true)
	if err != nil {
		t.Fatalf("emulatedBackend: %v", err)
	}
	if node.SPI != other {
		t.Errorf("the build asked for controller %d and got %d", other, node.SPI)
	}
	// The coprocessor setting travels the same way, and is off until asked for.
	if node.CoprocAtReset {
		t.Error("a build that asked for nothing got enabled coprocessors")
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
	if node.SPI != declared.QEMU.SPI {
		t.Errorf("clearing the controller left it on %d rather than the board's %d",
			node.SPI, declared.QEMU.SPI)
	}
}
