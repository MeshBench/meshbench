package firmware_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/mesh/firmware"
)

// Padding is where two traps live, and both produce failures that name the
// wrong thing.
//
// QEMU accepts only 2, 4, 8 or 16 MB images, and the size has to match what the
// image header asks for or ESP-IDF asserts inside do_core_init. The header sits
// at 0x1000 in a merged image because the file begins with padding; read it
// from zero and the answer is 0xff, which is how a working image gets padded to
// the wrong size and blamed on the firmware.
func TestPadImageReadsTheHeaderWhereItActuallyIs(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "merged.bin")
	dst := filepath.Join(dir, "padded.bin")

	// A merged image: padding, then the header at 0x1000 declaring 4 MB.
	img := make([]byte, 0x2000)
	for i := range img[:0x1000] {
		img[i] = 0xFF
	}
	img[0x1000] = 0xE9 // magic
	img[0x1003] = 0x20 // high nibble 2 -> 4 MB
	if err := os.WriteFile(src, img, 0o644); err != nil {
		t.Fatal(err)
	}

	mb, err := firmware.PadImage(src, dst)
	if err != nil {
		t.Fatal(err)
	}
	if mb != 4 {
		t.Errorf("read %d MB from the header, want 4", mb)
	}
	st, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() != 4<<20 {
		t.Errorf("padded to %d bytes, want %d", st.Size(), 4<<20)
	}

	// The padding must be erased-flash, not zeroes: a bootloader that reads
	// past the image should see an empty chip.
	out, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if out[len(out)-1] != 0xFF {
		t.Errorf("padded with 0x%02x, want 0xff", out[len(out)-1])
	}
}

// An application-only .bin is the file most people reach for and it boots
// nothing. Saying so here is much better than a watchdog reset later.
func TestPadImageRejectsAnApplicationOnlyBuild(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "app.bin")
	// No 0xE9 at 0x1000: this is an application, not a flash image.
	if err := os.WriteFile(src, make([]byte, 0x4000), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := firmware.PadImage(src, filepath.Join(dir, "out.bin"))
	if err == nil {
		t.Fatal("an application-only image was accepted as bootable")
	}
	if !strings.Contains(err.Error(), "merged") {
		t.Errorf("the error does not say what is wrong with it: %v", err)
	}
}

// A missing emulator should send someone to the right place. "not found" alone
// sends them to their package manager, and a distribution build has no SX1262.
func TestMissingToolsExplainThemselves(t *testing.T) {
	t.Setenv(firmware.EnvQEMU, filepath.Join(t.TempDir(), "not-here"))
	n := &firmware.EmulatedNode{
		Image: filepath.Join(t.TempDir(), "image.bin"), NodeName: "n1",
	}
	if err := os.WriteFile(n.Image, make([]byte, 16), 0o644); err != nil {
		t.Fatal(err)
	}
	err := n.Start(t.Context(), "")
	if err == nil {
		t.Fatal("started with no emulator present")
	}
	if !strings.Contains(err.Error(), firmware.EnvQEMU) &&
		!strings.Contains(err.Error(), firmware.EnvRadioServer) {
		t.Errorf("the error names neither environment variable: %v", err)
	}
}

func TestEmulatedNodeNeedsAnImage(t *testing.T) {
	n := &firmware.EmulatedNode{NodeName: "n1"}
	if err := n.Start(t.Context(), ""); err == nil {
		t.Error("started with no flash image")
	}
}
