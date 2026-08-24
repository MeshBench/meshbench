package firmware

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

// The ESP-IDF partition table: a fixed offset, 32-byte entries, each starting
// with a magic the table ends without.
const (
	partitionTableOffset = 0x8000
	partitionEntrySize   = 32
	partitionEntries     = 95
)

var partitionMagic = [2]byte{0xAA, 0x50}

// padToDeclaredFlash writes a copy of an ESP32 flash image grown to the size
// its own partition table describes, and returns the path to it.
//
// A published image carries only the bytes that were programmed - bootloader,
// table and application - and stops there. The partitions after the
// application are empty on a real board, so nothing is lost by leaving them
// out of a file; but a file is exactly as long as it is, and an emulator given
// one as a flash device has a flash chip that ends early. On this board the
// image runs to 0x127FC0 while the table places SPIFFS at 0x290000, so the
// filesystem is not merely empty - it is off the end of the chip. It fails to
// mount, fails to format, and every preference the firmware wants to read or
// write fails with it.
//
// Padded rather than reported, because the image is not wrong: it is a
// programming image, and a flash device is what we are asking it to be.
func padToDeclaredFlash(image, dir string) (string, error) {
	b, err := os.ReadFile(image)
	if err != nil {
		return "", fmt.Errorf("firmware: reading the flash image: %w", err)
	}
	want := declaredFlashSize(b)
	if want <= len(b) {
		return image, nil
	}
	out := filepath.Join(dir, "flash.bin")
	grown := make([]byte, want)
	copy(grown, b)
	// Erased flash is all ones, and SPIFFS reads a zeroed region as a
	// corrupt filesystem rather than a blank one.
	for i := len(b); i < want; i++ {
		grown[i] = 0xFF
	}
	if err := os.WriteFile(out, grown, 0o644); err != nil {
		return "", fmt.Errorf("firmware: writing the padded flash image: %w", err)
	}
	return out, nil
}

// HasPartitionTable reports whether a flash image carries a partition table
// where the ROM bootloader looks for one.
//
// This is what separates a whole flash image from the application on its own,
// and it is not the image header: an application image begins with 0xE9 too,
// because it is an ESP image as well - just one that starts at 0x10000 rather
// than at the beginning of the chip. Both halves of a published release are
// called .bin, both start with the same magic, and only one of them boots.
// Measured on Meshtastic's own pair, where firmware-heltec-v3-2.7.26.bin and
// firmware-heltec-v3-2.7.26.factory.bin differ in exactly this.
func HasPartitionTable(b []byte) bool {
	if len(b) < partitionTableOffset+partitionEntrySize {
		return false
	}
	e := b[partitionTableOffset:]
	return e[0] == partitionMagic[0] && e[1] == partitionMagic[1]
}

// declaredFlashSize is the end of the last partition the table lists, or zero
// if there is no table to read.
func declaredFlashSize(b []byte) int {
	end := 0
	for i := 0; i < partitionEntries; i++ {
		off := partitionTableOffset + i*partitionEntrySize
		if off+partitionEntrySize > len(b) {
			break
		}
		e := b[off : off+partitionEntrySize]
		if e[0] != partitionMagic[0] || e[1] != partitionMagic[1] {
			break
		}
		at := int(binary.LittleEndian.Uint32(e[4:8]))
		size := int(binary.LittleEndian.Uint32(e[8:12]))
		if at+size > end {
			end = at + size
		}
	}
	return end
}
