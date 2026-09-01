package emulated

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
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

// ImageFacts is what can be told about a build by reading the front of it.
//
// For a window that has to say why a build will not run, in the place somebody
// is looking at that build, rather than in a refusal several minutes into a
// start. Every field is what was read, not what should be done about it.
type ImageFacts struct {
	// Kind is the one-line answer: a whole flash image, an application on its
	// own, or something this cannot read.
	Kind string
	// Bootable reports whether a board could start from it - a header and a
	// partition table, which is what the ROM bootloader needs.
	Bootable bool
	// FlashMB is the chip size the header declares, or zero when there is no
	// header to read it from.
	FlashMB int
}

// imageHeadBytes is how much of a build is read to answer that.
//
// The partition table sits at 0x8000 and is 0xC00 long, so this covers the
// header, the table and a little past it. Reading the whole build instead
// would mean holding sixteen megabytes per row of a library that may have
// thirty of them, every time the library is rebuilt.
const imageHeadBytes = partitionTableOffset + 0x1000

// InspectImage reads the front of a build and says what it is.
//
// An unreadable file is not an error worth propagating: the caller is a
// library row, and "this cannot be read" is the answer it wants to show.
func InspectImage(path string) ImageFacts {
	f, err := os.Open(path) //nolint:gosec // a path this package composed, from its own cache
	if err != nil {
		return ImageFacts{Kind: "unreadable"}
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, imageHeadBytes)
	n, err := io.ReadFull(f, head)
	if err != nil && n == 0 {
		return ImageFacts{Kind: "unreadable"}
	}
	head = head[:n]
	mb, err := ClassifyESPImage(head)
	if err != nil {
		return ImageFacts{Kind: "not an ESP32 flash image"}
	}
	if !HasPartitionTable(head) {
		// The distinction that decides whether a board starts: the header is
		// the same byte in both, and only the table at 0x8000 tells them
		// apart.
		return ImageFacts{Kind: "application only - no partition table", FlashMB: mb}
	}
	return ImageFacts{Kind: "whole flash image", Bootable: true, FlashMB: mb}
}

// PadImageKeeping is PadImage, except that a flash the node has already been
// running is left exactly as it is.
//
// A board keeps what it was told between runs, and this is where that had
// stopped being true. The flash was rewritten from the pristine image on every
// start, so an emulated node's NVS and its filesystem were blanked each time -
// its identity, its preferences, its contacts, its region. Two places in the
// tree describe the opposite behaviour and are right about native nodes: a
// repeater keeping its identity between sessions is how hardware behaves, and
// firmware.wipe exists precisely because it does.
//
// What decides is the source, recorded beside the flash. A node pinned to a
// different build gets a fresh chip, because that is what reflashing a board
// is; a node started again on the same build gets the chip it left behind.
// Same reasoning as the card image, which has always been kept.
func PadImageKeeping(src, dst string) (int, error) {
	stamp := dst + ".src"
	want, err := imageStamp(src)
	if err != nil {
		return 0, err
	}
	if have, err := os.ReadFile(stamp); err == nil && string(have) == want {
		if st, err := os.Stat(dst); err == nil && st.Size() > 0 {
			return int(st.Size() >> 20), nil
		}
	}
	mb, err := PadImage(src, dst)
	if err != nil {
		return 0, err
	}
	// Written after the flash, so an interrupted write leaves a stamp that
	// does not match and the next run rebuilds rather than booting half a
	// chip. Best effort: a stamp that cannot be written costs a re-flash on
	// the next start, which is what happened every time before this.
	_ = os.WriteFile(stamp, []byte(want), 0o644)
	return mb, nil
}

// imageStamp identifies the build a flash was made from.
//
// The digest rather than the path or the modification time: a build imported
// twice under two labels is two paths and one chip, and an image rebuilt in
// place by a compiler keeps both its path and, often enough, its size.
func imageStamp(src string) (string, error) {
	// The image this node was told to run, from the cache or from an import.
	f, err := os.Open(src) //nolint:gosec // the build the caller asked for
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// PadImage copies a flash image padded to a size QEMU will accept, always
// rewriting the destination.
//
// Two traps in one function. QEMU takes only 2, 4, 8 or 16 MB images; and the
// size must match what the image header asks for, or ESP-IDF asserts in
// do_core_init with a message naming both sizes.
//
// Callers starting a node want PadImageKeeping instead: this one blanks
// whatever the board had written to its flash.
func PadImage(src, dst string) (int, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return 0, err
	}
	mb, err := ClassifyESPImage(data)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", src, err)
	}
	want := mb << 20
	if len(data) > want {
		return 0, fmt.Errorf("firmware: %s is larger than the %d MB its header declares",
			src, mb)
	}
	out := make([]byte, want)
	copy(out, data)
	for i := len(data); i < want; i++ {
		out[i] = 0xFF // erased flash
	}
	return mb, os.WriteFile(dst, out, 0o644)
}

// ClassifyESPImage reads an ESP32 flash image's header and answers the flash
// size in megabytes it was built for, or says why it is not one.
//
// Split out of PadImage so the same question can be asked at import, where it
// can still be answered by refusing. Asked only at play, an application-only
// build imported cleanly, listed cleanly, could be pinned to a node - and then
// failed minutes later, in a message about a flash image, to somebody who
// thought they were starting a board.
func ClassifyESPImage(data []byte) (flashMB int, err error) {
	if len(data) < 0x1004 {
		return 0, fmt.Errorf("firmware: too small to be a merged image")
	}
	// Where the header lives differs by part, so it is looked for rather than
	// assumed: an ESP32 boots its bootloader from 0x1000 and a merged image
	// for one starts with padding, while an ESP32-S3 boots from zero. The byte
	// is 0xE9 either way.
	hdr := -1
	switch {
	case data[0] == 0xE9:
		hdr = 0 // ESP32-S3, and the other parts that boot from zero
	case data[0x1000] == 0xE9:
		hdr = 0x1000 // ESP32
	}
	if hdr < 0 {
		return 0, fmt.Errorf("firmware: no image header at 0x0 or 0x1000; " +
			"it is probably an application-only build rather than a merged one")
	}
	sizes := map[byte]int{0: 1, 1: 2, 2: 4, 3: 8, 4: 16}
	mb, ok := sizes[data[hdr+3]>>4]
	if !ok {
		return 0, fmt.Errorf("firmware: the image header declares an unknown flash size")
	}
	if mb == 1 {
		mb = 2 // QEMU's smallest
	}
	return mb, nil
}
