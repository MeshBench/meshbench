package firmware_test

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/mesh/firmware"
)

// Real asset names from repeater-v1.17.0. The catalogue is the naming scheme,
// so a parser that is nearly right offers builds that cannot be run.
func TestParsesPublishedAssetNames(t *testing.T) {
	cases := []struct {
		name      string
		board     string
		role      string
		transport string
		version   string
		merged    bool
		format    string
	}{
		{"Heltec_v3_repeater-v1.17.0-727fc05-merged.bin",
			"Heltec_v3", "simple_repeater", "", "v1.17.0", true, "bin"},
		{"Heltec_v3_repeater-v1.17.0-727fc05.bin",
			"Heltec_v3", "simple_repeater", "", "v1.17.0", false, "bin"},
		{"RAK_4631_repeater-v1.17.0-727fc05.uf2",
			"RAK_4631", "simple_repeater", "", "v1.17.0", false, "uf2"},
		// Boards with hyphens and digits, which is why the split anchors on the
		// role rather than counting separators.
		{"Ebyte_EoRa-S3_Repeater-v1.17.0-727fc05-merged.bin",
			"Ebyte_EoRa-S3", "simple_repeater", "", "v1.17.0", true, "bin"},
		{"Generic_E22_sx1262_repeater-v1.17.0-727fc05-merged.bin",
			"Generic_E22_sx1262", "simple_repeater", "", "v1.17.0", true, "bin"},
		{"GAT562_Mesh_EVB_Pro_repeater-v1.17.0-727fc05.uf2",
			"GAT562_Mesh_EVB_Pro", "simple_repeater", "", "v1.17.0", false, "uf2"},
		// The tags say repeater and companion; the applications are called
		// simple_repeater and companion_radio, and those are the names the
		// runner and the board wiring use.
		//
		// A companion carries its transport in the same field, and every
		// published companion does. This test previously used an invented
		// "..._companion-v1.17.0..." that upstream has never released, so the
		// parser matched the name nobody publishes and dropped all 74 real
		// ones - the firmware library listed no companion builds at all.
		{"Heltec_v3_companion_radio_usb-v1.17.0-727fc05-merged.bin",
			"Heltec_v3", "companion_radio", "usb", "v1.17.0", true, "bin"},
		{"Heltec_v3_companion_radio_ble-v1.17.0-727fc05-merged.bin",
			"Heltec_v3", "companion_radio", "ble", "v1.17.0", true, "bin"},
		{"heltec_rc32_without_display_companion_radio_usb-v1.17.0-727fc05-merged.bin",
			"heltec_rc32_without_display", "companion_radio", "usb", "v1.17.0", true, "bin"},
		{"Heltec_v3_room_server-v1.17.0-727fc05-merged.bin",
			"Heltec_v3", "simple_room_server", "", "v1.17.0", true, "bin"},
	}

	for _, c := range cases {
		got, ok := firmware.ParseAssetName(c.name)
		if !ok {
			t.Errorf("%s was not recognised as a published asset", c.name)
			continue
		}
		if got.Board != c.board {
			t.Errorf("%s: board %q, want %q", c.name, got.Board, c.board)
		}
		if got.Role != c.role {
			t.Errorf("%s: role %q, want %q", c.name, got.Role, c.role)
		}
		if got.Transport != c.transport {
			t.Errorf("%s: transport %q, want %q", c.name, got.Transport, c.transport)
		}
		if got.Version != c.version {
			t.Errorf("%s: version %q, want %q", c.name, got.Version, c.version)
		}
		if got.Merged != c.merged {
			t.Errorf("%s: merged %v, want %v", c.name, got.Merged, c.merged)
		}
		if got.Format != c.format {
			t.Errorf("%s: format %q, want %q", c.name, got.Format, c.format)
		}
	}
}

func TestIgnoresThingsThatAreNotBoardImages(t *testing.T) {
	for _, name := range []string{
		"meshcore-simple_repeater-linux-amd64", // a native build
		"source.tar.gz",
		"README.md",
	} {
		if _, ok := firmware.ParseAssetName(name); ok {
			t.Errorf("%s was read as a board image", name)
		}
	}
}

// Offering an image that cannot boot is worse than not offering it: the failure
// arrives when someone presses run, by which point they have stopped thinking
// about the picker.
func TestRunnableKeepsOnlyWhatCouldBoot(t *testing.T) {
	all := []firmware.BoardImage{
		{Board: "Generic_E22_sx1262", Merged: true, Format: "bin"},
		{Board: "Generic_E22_sx1262", Merged: false, Format: "bin"}, // app only
		{Board: "RAK_4631", Merged: false, Format: "uf2"},           // nRF52
		{Board: "Heltec_v3", Merged: true, Format: "bin"},           // no wiring
	}
	wired := func(board string) bool { return board == "Generic_E22_sx1262" }

	got := firmware.Runnable(all, wired)
	if len(got) != 1 {
		for _, g := range got {
			t.Logf("  kept %s merged=%v %s", g.Board, g.Merged, g.Format)
		}
		t.Fatalf("kept %d images, want 1", len(got))
	}
	if got[0].Board != "Generic_E22_sx1262" || !got[0].Merged {
		t.Errorf("kept the wrong image: %+v", got[0])
	}
}

// A bare .bin is the application without a bootloader. It is the file most
// people reach for and it boots nothing, so it must never be offered as though
// it would.
func TestBareImageIsNeverRunnable(t *testing.T) {
	got := firmware.Runnable([]firmware.BoardImage{
		{Board: "Generic_E22_sx1262", Merged: false, Format: "bin"},
	}, nil)
	if len(got) != 0 {
		t.Error("an application-only image was offered as runnable")
	}
}

// The two companion builds of one board and version must not share a file.
//
// They did: the cache path was role plus version, and the transport is what
// separates them. Whichever was downloaded second overwrote the first, so a
// node asking for the USB build could silently run the BLE one and wait for a
// phone that has no way to arrive.
func TestCompanionTransportsDoNotShareACachePath(t *testing.T) {
	usb, _ := firmware.ParseAssetName("Heltec_v3_companion_radio_usb-v1.17.0-727fc05-merged.bin")
	ble, _ := firmware.ParseAssetName("Heltec_v3_companion_radio_ble-v1.17.0-727fc05-merged.bin")
	if a, b := firmware.BoardImagePath("/c", usb), firmware.BoardImagePath("/c", ble); a == b {
		t.Errorf("both companion builds cache to %s", a)
	}
}

// A BLE companion boots and then waits for a phone. There is no Bluetooth here,
// so offering it produces a node that looks hung rather than one that failed.
func TestRunnableDropsBluetoothCompanions(t *testing.T) {
	all := []firmware.BoardImage{}
	for _, n := range []string{
		"Heltec_v3_companion_radio_usb-v1.17.0-727fc05-merged.bin",
		"Heltec_v3_companion_radio_ble-v1.17.0-727fc05-merged.bin",
	} {
		img, ok := firmware.ParseAssetName(n)
		if !ok {
			t.Fatalf("%s did not parse", n)
		}
		all = append(all, img)
	}
	got := firmware.Runnable(all, nil)
	if len(got) != 1 || got[0].Transport != "usb" {
		t.Errorf("Runnable kept %+v; only the usb companion can run here", got)
	}
}
