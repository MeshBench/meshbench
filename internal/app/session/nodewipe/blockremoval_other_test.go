//go:build !windows

package nodewipe_test

import (
	"os"
	"path/filepath"
	"testing"
)

// blockRemoval makes a file impossible to delete, the way this platform does
// it: a directory nobody may write to.
//
// Holding the file open would not work here - Unix unlinks an open file
// happily, which is the whole difference from Windows.
func blockRemoval(t *testing.T, path string) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}
