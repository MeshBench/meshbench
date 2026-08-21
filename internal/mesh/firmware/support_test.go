package firmware

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A missing model must be named before Renode is started, not discovered as a
// compiler error inside a console log. The board is not at fault and the
// message must not read as though it is.
func TestCheckSupportNamesWhatIsMissing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "twim.repl"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := CheckSupport(dir, "twim.repl", "cryptocell.repl", "saadc.repl")
	if err == nil {
		t.Fatal("two absent files accepted")
	}
	for _, want := range []string{"cryptocell.repl", "saadc.repl", "2 of 3", EnvRenodeSupport} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message does not mention %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "twim.repl") {
		t.Errorf("names a file that is present: %v", err)
	}
}

func TestCheckSupportPassesWhenComplete(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "peripherals"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"twim.repl", "peripherals/NRF52840_TWIM.cs"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := CheckSupport(dir, "twim.repl", "peripherals/NRF52840_TWIM.cs"); err != nil {
		t.Errorf("complete directory refused: %v", err)
	}
}

// The environment variable is what a source tree uses, and it wins: the cache
// it would otherwise fall back to is the one place nothing fills.
func TestSupportDirPrefersTheEnvironment(t *testing.T) {
	t.Setenv(EnvRenodeSupport, "/somewhere/of/my/own")
	if got := SupportDir(); got != "/somewhere/of/my/own" {
		t.Errorf("SupportDir() = %q, want the environment's answer", got)
	}
}

func TestSupportDirFallsBackToTheCache(t *testing.T) {
	t.Setenv(EnvRenodeSupport, "")
	if got := SupportDir(); got != ToolsDir() {
		t.Errorf("SupportDir() = %q, want %q", got, ToolsDir())
	}
}
