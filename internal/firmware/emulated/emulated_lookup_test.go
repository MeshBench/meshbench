package emulated

// White-box, because lookupTool is where the bundles are found or not found and
// the layouts it has to know about are not visible from outside the package.

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/MeshBench/meshbench/internal/firmware/emulated/renode"
)

// Renode unpacks into renode_<version>-portable/, and a zip cannot carry the
// symlink the Linux tarball and the macOS bundle use. The Windows bundle
// therefore ships an emulator that was found by nobody until the versioned
// directory was searched too: the workbench reported "renode not found" with
// renode.exe sitting two directories away.
func TestLookupToolFindsRenodeInItsVersionedDirectory(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Skipf("no executable path here: %v", err)
	}
	name := "renode"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	// Beside the test binary, because that is the directory lookupTool looks
	// in - the same rule the shipped binary is subject to.
	dir := filepath.Join(filepath.Dir(self), "renode_1.16.1-portable")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Skipf("cannot write beside the test binary: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	bin := filepath.Join(dir, name)
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// The environment variable would short-circuit the search this is testing.
	t.Setenv(renode.EnvRenode, "")

	got, err := lookupTool(renode.EnvRenode, "renode")
	if err != nil {
		t.Fatalf("renode not found with %s present: %v", bin, err)
	}
	if got != bin {
		t.Errorf("found %s, wanted the bundled %s", got, bin)
	}
}
