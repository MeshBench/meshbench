package emulated

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// A published image stops after the application, so the partitions the table
// places beyond it are off the end of the chip rather than merely empty. The
// board this was found on could not mount its filesystem, could not format it
// either, and failed every preference read and write in consequence.
func TestAFlashImageGrowsToWhatItsTableDeclares(t *testing.T) {
	img := make([]byte, 0x20000)
	entry := func(i int, at, size uint32, label string) {
		off := partitionTableOffset + i*partitionEntrySize
		img[off], img[off+1] = partitionMagic[0], partitionMagic[1]
		binary.LittleEndian.PutUint32(img[off+4:], at)
		binary.LittleEndian.PutUint32(img[off+8:], size)
		copy(img[off+12:off+28], label)
	}
	entry(0, 0x9000, 0x5000, "nvs")
	entry(1, 0x10000, 0x140000, "app0")
	entry(2, 0x290000, 0x160000, "spiffs")

	dir := t.TempDir()
	src := filepath.Join(dir, "published.bin")
	if err := os.WriteFile(src, img, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := padToDeclaredFlash(src, dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 0x3F0000 {
		t.Fatalf("padded to %#x, want %#x: the filesystem still ends up off the chip", len(b), 0x3F0000)
	}
	// Erased flash is ones. Zeroes read as a corrupt filesystem, not a blank one.
	if b[0x290000] != 0xFF {
		t.Errorf("padding byte is %#x, want 0xFF", b[0x290000])
	}
	if string(b[:0x20000]) != string(img) {
		t.Error("the programmed bytes changed")
	}
}

// An image with no table is left exactly as it is.
func TestAnImageWithNoPartitionTableIsUntouched(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "bare.bin")
	if err := os.WriteFile(src, make([]byte, 1024), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := padToDeclaredFlash(src, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != src {
		t.Fatalf("returned %s, want the original %s", got, src)
	}
}
