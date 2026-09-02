package nodewipe_test

import (
	"os"
	"testing"
	"time"
)

// nodeFSRoot is where nodes under test keep their files.
//
// Not t.TempDir(), which fails the test if its own cleanup cannot unlink. A
// node holds a .lock while it is alive and for a moment after it is killed,
// and Windows refuses to remove an open file where Unix removes it happily.
func nodeFSRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "mb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for range 20 {
			if err := os.RemoveAll(dir); err == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Logf("could not remove %s; something is still holding a file in it", dir)
	})
	return dir
}
