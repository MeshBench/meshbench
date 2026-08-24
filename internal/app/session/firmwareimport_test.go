package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// espImage writes an ESP32 flash image: a header where the part boots from,
// declaring the given flash size, and a partition table where the ROM
// bootloader looks for one unless withTable is false.
//
// Both halves matter and only together. An application-only build has the same
// header - it is an ESP image too - and differs precisely in having no table.
func espImage(t *testing.T, dir, name string, headerAt int, sizeCode byte, withTable bool) string {
	t.Helper()
	b := make([]byte, 0x12000)
	for i := range b {
		b[i] = 0xFF
	}
	b[headerAt] = 0xE9
	b[headerAt+3] = sizeCode << 4
	if withTable {
		b[0x8000], b[0x8001] = 0xAA, 0x50
	} else {
		b[0x8000], b[0x8001] = 0x12, 0x34 // application bytes, not a table
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Half a firmware is refused at the door.
//
// A published release for an ESP32 board carries two files whose names differ
// by one word: firmware-heltec-v3-2.7.26.bin is the application, and
// firmware-heltec-v3-2.7.26.factory.bin is the whole flash. A board starts from
// the bootloader, so only the second one boots - and the first one used to
// import cleanly, list cleanly, be pinnable to a node, and fail several minutes
// later in a message about flash images.
func TestAnApplicationOnlyBuildIsRefusedAtImport(t *testing.T) {
	dir := t.TempDir()

	// The application on its own, shaped like the real ones: the same image
	// header, and no partition table behind it.
	appOnly := espImage(t, dir, "firmware-heltec-v3-2.7.26.bin", 0, 3, false)
	err := refuseHalfAnImage(appOnly, "Heltec_v3")
	if err == nil {
		t.Fatal("an application-only build was accepted for an ESP32 board")
	}
	// The refusal has to name the way out, or it is a wall.
	for _, want := range []string{"application", "partition table", "factory.bin"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q: %v", want, err)
		}
	}

	// The whole flash image is accepted. An ESP32-S3 boots from zero.
	merged := espImage(t, dir, "firmware-heltec-v3-2.7.26.factory.bin", 0, 3, true)
	if err := refuseHalfAnImage(merged, "Heltec_v3"); err != nil {
		t.Errorf("a merged image was refused: %v", err)
	}

	// A host build has no board and is not a flash image at all.
	if err := refuseHalfAnImage(appOnly, ""); err != nil {
		t.Errorf("a build for this machine was judged as a flash image: %v", err)
	}

	// And an nRF52 board's image is not judged by Espressif's header. The
	// check would refuse every one of them, which is worse than not checking:
	// it would turn away the images that do work.
	if err := refuseHalfAnImage(appOnly, "RAK_4631"); err != nil {
		t.Errorf("an nRF52 image was judged by an ESP32 header: %v", err)
	}

	// A .uf2 carries its own addresses, so the question does not arise.
	uf2 := filepath.Join(dir, "build.uf2")
	if err := os.WriteFile(uf2, make([]byte, 0x4000), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := refuseHalfAnImage(uf2, "Heltec_v3"); err != nil {
		t.Errorf("a .uf2 was judged as a bare ESP32 application: %v", err)
	}
}

// The real files this issue was filed on, when they are on this machine.
//
// Skipped where they are not, because they are somebody's downloads rather
// than a fixture - but where they are, this is the measurement rather than a
// model of it: two Meshtastic releases, a Rust firmware and a third-party
// build, and the rule has to sort them the way esptool would.
func TestTheRealReleaseImagesAreSortedCorrectly(t *testing.T) {
	root := os.Getenv("HOME") + "/Documents/meshcore-ideas/firmware-bins"
	if _, err := os.Stat(root); err != nil {
		t.Skip("the firmware collection is not on this machine")
	}
	for _, c := range []struct {
		file     string
		board    string
		bootable bool
	}{
		{"Heltec-V3/Meshtastic/firmware-heltec-v3-2.7.26.54e0d8d.bin", "Heltec_v3", false},
		{"Heltec-V3/Meshtastic/firmware-heltec-v3-2.7.26.54e0d8d.factory.bin", "Heltec_v3", true},
		{"T-Deck/meshtastic/firmware-t-deck-2.7.26.54e0d8d.bin", "LilyGo_TDeck", false},
		{"T-Deck/meshtastic/firmware-t-deck-2.7.26.54e0d8d.factory.bin", "LilyGo_TDeck", true},
		{"T-Deck/mesh-rs/mesh-rs.bin", "LilyGo_TDeck", true},
		{"T-Deck/sigurdOS/firmware.bin", "LilyGo_TDeck", false},
		{"T-Deck/sigurdOS/firmware-merged.bin", "LilyGo_TDeck", true},
	} {
		path := filepath.Join(root, c.file)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		err := refuseHalfAnImage(path, c.board)
		if c.bootable && err != nil {
			t.Errorf("%s is a whole flash image and was refused: %v", c.file, err)
		}
		if !c.bootable && err == nil {
			t.Errorf("%s is the application on its own and was accepted", c.file)
		}
	}
}
