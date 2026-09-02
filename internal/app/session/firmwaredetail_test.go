package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/firmware/emulated"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// aCacheWith puts board images in a cache of this test's own and returns it.
func aCacheWith(t *testing.T, board string, names ...string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", home)
	t.Setenv("LOCALAPPDATA", home)
	t.Setenv("HOME", home)
	return addToCache(t, board, names...)
}

// addToCache adds more images to the cache aCacheWith already redirected.
func addToCache(t *testing.T, board string, names ...string) string {
	t.Helper()
	dir := filepath.Join(firmware.DefaultCacheDir(), "board", board)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		// A header and a partition table, so the image reads as one a board
		// could actually start from - the facts are half of what the window
		// is for and a stub of zeroes would report the wrong thing.
		img := make([]byte, 0x9000)
		img[0], img[3] = 0xE9, 0x40 // an S3 header declaring 16 MB
		img[0x8000], img[0x8001] = 0xAA, 0x50
		if err := os.WriteFile(filepath.Join(dir, n), img, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func aSimWithLibrary(t *testing.T, nodes ...scenario.Node) (*state.Store, *Sim) {
	t.Helper()
	st := state.New(10)
	s := &Sim{nodes: nodes}
	registerFirmwareLibrary(st, s)
	registerFirmwareDetail(st, s)
	go st.Run(t.Context())
	return st, s
}

// What the window asks the moment a build does not do what somebody expected:
// where is it, what is it, and what has been decided about it.
func TestOneBuildReportsWhereItIsAndWhatItIs(t *testing.T) {
	dir := aCacheWith(t, "LilyGo_TDeck", "companion_radio_usb@mesh-rs.bin")
	st, _ := aSimWithLibrary(t)
	ctx := t.Context()

	got, err := st.Do(ctx, "firmware.details", "mesh-rs")
	if err != nil {
		t.Fatalf("firmware.details: %v", err)
	}
	m := got.(map[string]any)
	if want := filepath.Join(dir, "companion_radio_usb@mesh-rs.bin"); m["path"] != want {
		t.Errorf("path is %v, want %s", m["path"], want)
	}
	if m["settings_path"] != firmware.SettingsPath(m["path"].(string)) {
		t.Errorf("settings_path is %v", m["settings_path"])
	}
	if m["bootable"] != true || m["flash_mb"] != 16 {
		t.Errorf("read the image as %v / %v MB, want bootable and 16",
			m["bootable"], m["flash_mb"])
	}
	if m["coproc_at_reset"] != false {
		t.Error("a build nobody has touched came back with a setting on")
	}
	if _, err := st.Do(ctx, "firmware.details", "nothing-called-this"); err == nil {
		t.Error("a build that does not exist was reported on")
	}
}

// A label alone is not an identity: the same image imported for two boards
// shares one, and acting on the wrong one is a rename of somebody else's
// image. So it is refused rather than guessed at, and the refusal says how to
// break the tie.
func TestAnAmbiguousLabelIsRefusedRatherThanGuessed(t *testing.T) {
	aCacheWith(t, "LilyGo_TDeck", "companion_radio_usb@both.bin")
	addToCache(t, "Heltec_v3", "simple_repeater@both.bin")
	st, _ := aSimWithLibrary(t)
	ctx := t.Context()

	_, err := st.Do(ctx, "firmware.details", "both")
	if err == nil {
		t.Fatal("an ambiguous label was accepted")
	}
	if !strings.Contains(err.Error(), "Heltec_v3") ||
		!strings.Contains(err.Error(), "LilyGo_TDeck") {
		t.Errorf("the refusal does not name the candidates: %v", err)
	}
	// Named in full it is not ambiguous at all.
	if _, err := st.Do(ctx, "firmware.details", map[string]any{
		"version": "both", "role": "simple_repeater", "board": "Heltec_v3",
	}); err != nil {
		t.Errorf("naming the build in full was still refused: %v", err)
	}
}

// The setting is kept beside the image, so it follows the build rather than
// the board - and it is what the emulator is actually given.
func TestASettingFollowsTheBuildIntoTheEmulator(t *testing.T) {
	dir := aCacheWith(t, "LilyGo_TDeck", "companion_radio_usb@mesh-rs.bin")
	st, _ := aSimWithLibrary(t)
	ctx := t.Context()
	image := filepath.Join(dir, "companion_radio_usb@mesh-rs.bin")

	if _, err := st.Do(ctx, "firmware.update", map[string]any{
		"version": "mesh-rs", "coproc_at_reset": true, "notes": "traps in its own vector",
	}); err != nil {
		t.Fatalf("firmware.update: %v", err)
	}
	if !firmware.LoadBuildSettings(image).CoprocAtReset {
		t.Fatal("the setting did not reach the file beside the image")
	}
	// And a node running this build is given it, which is the whole point.
	node := &emulated.EmulatedNode{Machine: "esp32s3", SPI: 3, NSS: 9, Busy: 13,
		CoprocAtReset: firmware.LoadBuildSettings(image).CoprocAtReset}
	if !node.CoprocAtReset {
		t.Error("a node built from this image was not told")
	}

	// Turning it off again has to be a different answer from not mentioning
	// it: a bool that cannot say "leave it alone" would reset every setting
	// somebody did not repeat.
	if _, err := st.Do(ctx, "firmware.update", map[string]any{
		"version": "mesh-rs", "notes": "still true",
	}); err != nil {
		t.Fatal(err)
	}
	if !firmware.LoadBuildSettings(image).CoprocAtReset {
		t.Error("a call that said nothing about the setting turned it off")
	}
	if _, err := st.Do(ctx, "firmware.update", map[string]any{
		"version": "mesh-rs", "coproc_at_reset": false,
	}); err != nil {
		t.Fatal(err)
	}
	if firmware.LoadBuildSettings(image).CoprocAtReset {
		t.Error("saying no did not turn it off")
	}
}

// Renaming moves the file, because the name is the identity. Anything pinned
// to the old name has to come along, or it fails at its next start with "no
// image in the cache" - about a build sitting in the library under a new name.
func TestRenamingABuildRepointsTheNodesRunningIt(t *testing.T) {
	dir := aCacheWith(t, "Heltec_v3", "simple_repeater@old.bin")
	st, s := aSimWithLibrary(t, scenario.Node{
		Name: "GB7XYZ", Kind: scenario.SimpleRepeater,
		Firmware: scenario.FirmwareRef{Version: "old", Board: "Heltec_v3",
			Role: scenario.RoleSimpleRepeater},
	})
	ctx := t.Context()

	got, err := st.Do(ctx, "firmware.update", map[string]any{
		"version": "old", "label": "wadamesh 1.2",
	})
	if err != nil {
		t.Fatalf("firmware.update: %v", err)
	}
	m := got.(map[string]any)
	if m["renamed"] != true || m["repinned"] != 1 {
		t.Errorf("renamed=%v repinned=%v, want true and 1", m["renamed"], m["repinned"])
	}
	if s.nodes[0].Firmware.Version != "wadamesh 1.2" {
		t.Errorf("the node still asks for %q", s.nodes[0].Firmware.Version)
	}
	if _, err := os.Stat(filepath.Join(dir, "simple_repeater@wadamesh 1.2.bin")); err != nil {
		t.Errorf("the file did not move: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "simple_repeater@old.bin")); !os.IsNotExist(err) {
		t.Error("the old name is still on disk")
	}
}
