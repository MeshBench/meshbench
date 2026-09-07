package firmwarelib

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/firmware/emulated"
)

// Upstream publishes both variants for every ESP32 board, named apart only by
// "-merged", and the catalogue's regex makes that an optional group - so both
// parse to the same board, role and version. The download took whichever the
// listing yielded first, which for most boards was the bare application: no
// bootloader, no partition table, and the ROM reads the application header as
// a bootloader and panics.
func TestOnlyABootableImageIsAChoice(t *testing.T) {
	both := []emulated.BoardImage{
		// The bare application first, as the listing yielded it.
		{Board: "Heltec_v3", Role: "repeater", Version: "v1.17.1",
			Format: "bin", Merged: false, Name: "Heltec_v3_repeater-v1.17.1.bin"},
		{Board: "Heltec_v3", Role: "repeater", Version: "v1.17.1",
			Format: "bin", Merged: true, Name: "Heltec_v3_repeater-v1.17.1-merged.bin"},
	}

	got := emulated.Runnable(both, nil)
	if len(got) != 1 {
		t.Fatalf("Runnable kept %d of the two variants", len(got))
	}
	if !got[0].Merged {
		t.Errorf("Runnable kept %q, which boots nothing", got[0].Name)
	}

	// The nRF52 images are not merged - bootloader and SoftDevice are supplied
	// separately - so requiring "merged" everywhere would reject every one.
	uf2 := []emulated.BoardImage{{
		Board: "RAK4631", Role: "repeater", Version: "v1.17.1",
		Format: "uf2", Merged: false, Name: "RAK4631_repeater-v1.17.1.uf2",
	}}
	if len(emulated.Runnable(uf2, nil)) != 1 {
		t.Error("Runnable rejected an nRF52 .uf2, which is how those are published")
	}
}
