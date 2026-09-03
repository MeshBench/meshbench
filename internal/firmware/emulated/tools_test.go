package emulated

import (
	"os"
	"path/filepath"
	"testing"
)

// The support directory is where Renode's platform descriptions and our own
// peripherals are, and a release bundle carries them beside the binary.
//
// It counts a directory only when the files are in it. An empty one beside the
// binary winning over a populated tools directory would take every nRF52 board
// down and look like a broken emulator rather than a missing file.
func TestSupportDirIgnoresADirectoryWithNothingInIt(t *testing.T) {
	empty := t.TempDir()
	if err := os.MkdirAll(filepath.Join(empty, "peripherals"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvRenodeSupport, "")
	if got := SupportDir(); got == empty {
		t.Error("an empty directory was accepted as the support directory")
	}

	full := t.TempDir()
	if err := os.MkdirAll(filepath.Join(full, "peripherals"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(full, "peripherals", "VirtualSX1262.cs"),
		[]byte("//"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvRenodeSupport, full)
	if got := SupportDir(); got != full {
		t.Errorf("the override was ignored: got %q, want %q", got, full)
	}
}
