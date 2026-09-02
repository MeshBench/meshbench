package nodewipe_test

import (
	"os"
	"testing"
)

// blockRemoval makes a file impossible to delete, the way Windows does it: an
// open handle.
//
// A read-only directory is what the other platforms use and it does nothing
// here - Windows does not take a directory's permissions as a veto on creating
// or removing what is inside it, and an elevated shell would ignore the ACL
// besides. So the wipe succeeded, the test failed, and it read as the partial
// wipe going unreported rather than as nothing having been blocked.
//
// Windows will not unlink a file somebody still has open, which is the same
// property that makes t.TempDir() cleanup fail against a dying child.
func blockRemoval(t *testing.T, path string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
}
