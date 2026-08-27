package firmware

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A build with nothing decided about it reads as defaults, whether or not
// anything was ever written - which is what every build imported before any of
// this existed looks like.
func TestABuildWithNoSettingsReadsAsDefaults(t *testing.T) {
	image := filepath.Join(t.TempDir(), "companion_radio_usb@thing.bin")
	if err := os.WriteFile(image, []byte{0xE9}, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadBuildSettings(image); !got.Zero() {
		t.Fatalf("a build nobody has touched reads as %+v, want defaults", got)
	}
	// And so does one whose settings file is unreadable: the answer to "what
	// has been decided" is "nothing" either way, and a library that refused to
	// list a build over a corrupt sidecar would be unusable for the one reason
	// nobody could see.
	if err := os.WriteFile(SettingsPath(image), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadBuildSettings(image); !got.Zero() {
		t.Fatalf("a corrupt settings file reads as %+v, want defaults", got)
	}
}

func TestSettingsSurviveARoundTrip(t *testing.T) {
	image := filepath.Join(t.TempDir(), "companion_radio_usb@mesh-rs.bin")
	if err := os.WriteFile(image, []byte{0xE9}, 0o644); err != nil {
		t.Fatal(err)
	}
	want := BuildSettings{CoprocAtReset: true, Notes: "traps in its own vector"}
	if err := SaveBuildSettings(image, want); err != nil {
		t.Fatal(err)
	}
	if got := LoadBuildSettings(image); got != want {
		t.Fatalf("read back %+v, want %+v", got, want)
	}
	// Back to defaults removes the file rather than writing an empty one, so
	// a build returned to its defaults is indistinguishable from one that
	// never had settings.
	if err := SaveBuildSettings(image, BuildSettings{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(SettingsPath(image)); !os.IsNotExist(err) {
		t.Fatalf("settings file still there after being emptied: %v", err)
	}
}

// The settings sit in the same directory as the builds, so the thing that
// lists builds has to know the difference. It did not, once.
func TestSettingsDoNotListAsBuilds(t *testing.T) {
	cache := t.TempDir()
	dir := filepath.Join(cache, "board", "LilyGo_TDeck")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	image := filepath.Join(dir, "companion_radio_usb@mesh-rs.bin")
	if err := os.WriteFile(image, []byte{0xE9}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveBuildSettings(image, BuildSettings{CoprocAtReset: true}); err != nil {
		t.Fatal(err)
	}
	got := ListInstalled(cache)
	if len(got) != 1 {
		t.Fatalf("listed %d builds, want 1: %+v", len(got), got)
	}
	if got[0].Version != "mesh-rs" || got[0].Role != "companion_radio_usb" {
		t.Fatalf("listed %+v, want the image itself", got[0])
	}
}

func TestRenameCarriesTheSettingsAndTidiesUp(t *testing.T) {
	cache := t.TempDir()
	dir := filepath.Join(cache, "board", "LilyGo_TDeck")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	image := filepath.Join(dir, "companion_radio_usb@old.bin")
	if err := os.WriteFile(image, []byte{0xE9, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveBuildSettings(image, BuildSettings{CoprocAtReset: true}); err != nil {
		t.Fatal(err)
	}
	in := Installed{Version: "old", Role: "companion_radio_usb",
		Board: "LilyGo_TDeck", Path: image}

	moved, err := Rename(cache, in, "simple_repeater", "new name", "Heltec_v3")
	if err != nil {
		t.Fatal(err)
	}
	if moved.Version != "new name" || moved.Role != "simple_repeater" ||
		moved.Board != "Heltec_v3" {
		t.Fatalf("renamed to %+v", moved)
	}
	if !LoadBuildSettings(moved.Path).CoprocAtReset {
		t.Error("the settings did not follow the build")
	}
	if _, err := os.Stat(image); !os.IsNotExist(err) {
		t.Error("the old image is still there")
	}
	// The board it left is empty now, and a board listed with nothing in it
	// reads as a board whose builds went missing.
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("the emptied board directory was left behind")
	}
}

func TestRenameRefusesWhatItCannotDoSafely(t *testing.T) {
	cache := t.TempDir()
	dir := filepath.Join(cache, "board", "Heltec_v3")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mine := filepath.Join(dir, "simple_repeater@mine.bin")
	theirs := filepath.Join(dir, "simple_repeater@theirs.bin")
	for _, p := range []string{mine, theirs} {
		if err := os.WriteFile(p, []byte{0xE9}, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	in := Installed{Version: "mine", Role: "simple_repeater",
		Board: "Heltec_v3", Path: mine}

	// Onto a name that is taken: silently replacing somebody else's build is
	// the one outcome nothing could undo.
	if _, err := Rename(cache, in, "simple_repeater", "theirs", "Heltec_v3"); err == nil {
		t.Error("renaming over an existing build was allowed")
	} else if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("refused with %v, which does not say the name is taken", err)
	}

	// A build for this machine is named after the host it runs on, so there
	// is nothing here that renaming would mean.
	native := Installed{Native: true, Version: "v1.17.1", Role: "simple_repeater",
		Path: filepath.Join(cache, "native", "v1.17.1", "meshcore-simple_repeater-linux-amd64")}
	if _, err := Rename(cache, native, "simple_repeater", "other", ""); err == nil {
		t.Error("a native build was renamed")
	}

	// And a path outside the cache, which is the guard that stops a mistake
	// relocating whatever it was handed.
	outside := Installed{Version: "x", Role: "simple_repeater", Board: "Heltec_v3",
		Path: filepath.Join(t.TempDir(), "elsewhere.bin")}
	if _, err := Rename(cache, outside, "simple_repeater", "x", "Heltec_v3"); err == nil {
		t.Error("a build outside the cache was renamed")
	}
}

// Deleting a build takes its settings with it. Left behind, they would be
// inherited by the next build imported under the same name - which is a build
// silently running with somebody else's answer.
func TestDeletingABuildTakesItsSettings(t *testing.T) {
	cache := t.TempDir()
	dir := filepath.Join(cache, "board", "Heltec_v3")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	image := filepath.Join(dir, "simple_repeater@thing.bin")
	if err := os.WriteFile(image, []byte{0xE9}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveBuildSettings(image, BuildSettings{Notes: "mine"}); err != nil {
		t.Fatal(err)
	}
	in := Installed{Version: "thing", Role: "simple_repeater",
		Board: "Heltec_v3", Path: image}
	if err := Remove(cache, in); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(SettingsPath(image)); !os.IsNotExist(err) {
		t.Error("the settings outlived the build they describe")
	}
}
