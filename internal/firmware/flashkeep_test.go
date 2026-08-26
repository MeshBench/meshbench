package firmware_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MeshBench/meshbench/internal/firmware"
)

// mergedImage writes a whole flash image declaring 2 MB.
func mergedImage(t *testing.T, path string, mark byte) {
	t.Helper()
	b := make([]byte, 0x12000)
	for i := range b {
		b[i] = 0xFF
	}
	b[0] = 0xE9
	b[3] = 1 << 4 // 2 MB
	b[0x8000], b[0x8001] = 0xAA, 0x50
	b[0x10000] = mark
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A board keeps what it was told between runs, and this is where that had
// stopped being true for emulated ones.
//
// The flash was rebuilt from the pristine image on every start, so an emulated
// node's NVS and filesystem were blanked each time: a node configured over its
// console reverted the moment somebody stopped and started it, and both arms of
// a comparison began factory-fresh whether or not that was intended. Two places
// in the tree describe the opposite and are right about native nodes.
func TestAFlashSurvivesARestartOnTheSameBuild(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "build.bin")
	dst := filepath.Join(dir, "flash.bin")
	mergedImage(t, src, 0x01)

	if _, err := firmware.PadImageKeeping(src, dst); err != nil {
		t.Fatal(err)
	}
	// What the board wrote while it was running: its identity, in the part of
	// the chip the application owns.
	flash, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	copy(flash[0x9000:], []byte("GB7XYZ-identity"))
	if err := os.WriteFile(dst, flash, 0o644); err != nil {
		t.Fatal(err)
	}

	// Started again on the same build.
	if _, err := firmware.PadImageKeeping(src, dst); err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(again[0x9000:0x900F]) != "GB7XYZ-identity" {
		t.Error("restarting the node blanked what it had stored; a board does not do that")
	}

	// A different build is a reflash, and a reflashed board does start fresh.
	mergedImage(t, src, 0x02)
	if _, err := firmware.PadImageKeeping(src, dst); err != nil {
		t.Fatal(err)
	}
	fresh, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(fresh[0x9000:0x900F]) == "GB7XYZ-identity" {
		t.Error("a new build kept the last one's flash; reflashing a board does not do that")
	}
	if fresh[0x10000] != 0x02 {
		t.Error("the new build was not written at all")
	}
}

// A flash whose stamp is there but whose image has gone is rebuilt rather than
// trusted: the two are written in that order precisely so an interrupted write
// cannot leave a node booting half a chip.
func TestAMissingFlashIsRebuiltDespiteItsStamp(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "build.bin")
	dst := filepath.Join(dir, "flash.bin")
	mergedImage(t, src, 0x01)
	if _, err := firmware.PadImageKeeping(src, dst); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dst); err != nil {
		t.Fatal(err)
	}
	if _, err := firmware.PadImageKeeping(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("the flash was not rebuilt: %v", err)
	}
}
