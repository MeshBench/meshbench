// White-box, because where a tool was found is decided by two paths that can
// contain each other, and the containment is the whole fault.
package resources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/firmware/emulated"
)

// A tool in the tools directory has to read as fetched even when the tools
// directory is underneath the binary's own.
//
// It is, whenever the binary is run out of /tmp, which is exactly what a
// freshly built one is: a radioserver that had just been downloaded reported
// itself as bundled beside the binary, which is the one distinction the row
// exists to make. Both paths are forced here so the case is reproduced rather
// than waited for.
func TestAFetchedToolIsNotMistakenForABundledOne(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip("no executable path on this platform")
	}
	t.Setenv("XDG_CACHE_HOME", filepath.Join(filepath.Dir(exe), "cachehome"))
	tools := emulated.ToolsDir()
	if !underDir(tools, filepath.Dir(exe)) {
		t.Fatalf("the case is not set up: %s is not under %s", tools, filepath.Dir(exe))
	}
	got := whereFound(filepath.Join(tools, "radioserver"))
	if !strings.Contains(got, "tools directory") {
		t.Errorf("a tool in %s reads as %q", tools, got)
	}
	if besideBinary(filepath.Join(tools, "radioserver")) {
		t.Error("a tool in the tools directory counts as bundled beside the binary")
	}
	// The other direction still has to work: a tool actually beside the binary
	// is what a release tarball ships, and calling that a fetch would tell
	// somebody to download what they already have.
	if !besideBinary(filepath.Join(filepath.Dir(exe), "renode")) {
		t.Error("a tool beside the binary does not count as bundled")
	}
}

// An empty directory matches nothing. It is the value os.Executable hands back
// when it fails, and a prefix test against "" is true of every path there is.
func TestAnEmptyDirectoryContainsNothing(t *testing.T) {
	if underDir("/usr/bin/qemu-system-xtensa", "") {
		t.Error("an empty directory contains a path")
	}
	if underDir("", "/usr/bin") {
		t.Error("a directory contains an empty path")
	}
}
