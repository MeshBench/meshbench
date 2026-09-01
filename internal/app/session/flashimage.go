// Whether the file somebody chose is a whole flash image.
//
// Its own file because it is a fact about ESP32 images rather than anything to
// do with the library it guards, and because the reason it is checked at import
// is longer than the check.
package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/firmware/emulated"
)

// isESP32Board reports whether this board's MCU is one whose flash images the
// check below can read. A board nobody has heard of is not one to judge.
func isESP32Board(board string) bool {
	b, err := hw.BoardByName(board)
	if err != nil {
		return false
	}
	return strings.HasPrefix(strings.ToUpper(b.MCU), "ESP32")
}

// refuseHalfAnImage turns away an application-only build before it is stored.
//
// A published release for an ESP32 board carries two files whose names differ
// by one word, and only one of them boots: firmware-heltec-v3-2.7.26.bin is the
// application, and firmware-heltec-v3-2.7.26.factory.bin is the whole flash -
// bootloader, partition table and application together. A board starts from the
// bootloader, so the first of those starts nothing.
//
// Checked here rather than at play, where it was checked before. The refusal
// arrived several minutes after the import, phrased as a flash-image error, to
// somebody who thought they were starting a board - and the library went on
// offering the build that could not run. Checked here it is an answer to the
// question being asked, at the moment it is asked.
//
// Only for a board with an ESP32-family MCU: the header this reads is Espressif's,
// and an nRF52 image is a different shape entirely.
func refuseHalfAnImage(path, board string) error {
	if board == "" || strings.ToLower(filepath.Ext(path)) != ".bin" {
		return nil
	}
	if !isESP32Board(board) {
		return nil
	}
	// The path is what somebody chose in a file dialog, and reading it is the
	// whole of what this function is for.
	data, err := os.ReadFile(path) //nolint:gosec // the caller's own chosen import
	if err != nil {
		return err
	}
	if _, err := emulated.ClassifyESPImage(data); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	// The header is not the test. An application image begins with the same
	// magic byte - it is an ESP image too, just one that belongs at 0x10000
	// rather than at the start of the chip. What a whole flash image has and
	// an application has not is the partition table the ROM bootloader reads.
	if !emulated.HasPartitionTable(data) {
		return fmt.Errorf(
			"%s has no partition table at 0x8000, so it is the application on its "+
				"own rather than a whole flash image: it starts at 0x10000 and a board "+
				"starts from the bootloader. The release this came from publishes the "+
				"whole flash beside it, named -merged.bin or .factory.bin",
			filepath.Base(path))
	}
	return nil
}
