package firmware_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/mesh/firmware"
)

// A live check that the published release and the simulator agree about names.
// Skipped without the flag: a unit test that needs the network is a flaky test.
func TestLiveNativeCatalogue(t *testing.T) {
	if os.Getenv("MESHCORESIM_LIVE") == "" {
		t.Skip("set MESHCORESIM_LIVE=1")
	}
	c := &firmware.NativeCatalogue{CacheDir: t.TempDir()}
	path, err := c.Ensure(context.Background(), "simple_repeater", "main")
	if err != nil {
		t.Fatal(err)
	}
	// The binary reports the firmware's own airtime for a 40-byte packet. If the
	// download were the wrong file, or not executable, this is where it shows.
	out, err := exec.Command(path, "--print-airtime", "40").Output()
	if err != nil {
		t.Fatalf("running the downloaded build: %v", err)
	}
	if strings.TrimSpace(string(out)) != "300" {
		t.Errorf("airtime for 40 bytes at SF10/250kHz = %q, want 300", strings.TrimSpace(string(out)))
	}
}
