//go:build !windows

package control

import (
	"io/fs"
	"os"
	"testing"
)

// isPrivate checks the guarantee the way this platform expresses it: mode
// bits, which the kernel enforces.
func isPrivate(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != want {
		t.Errorf("%s is %04o, want %04o", path, got, want)
	}
}
