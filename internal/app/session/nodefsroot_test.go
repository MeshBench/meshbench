package session

import (
	"os"
	"testing"
	"time"
)

// nodeFSRoot is where nodes under test keep their files.
//
// Not t.TempDir(), which fails the test if its own cleanup cannot unlink. A
// node holds a .lock and a stderr.log while it is alive and for a moment after
// it is killed, and Windows refuses to remove an open file where Unix removes
// it happily. What these tests assert is what the verbs answer, and they do;
// a file that takes another moment to close was failing them on nothing else.
func nodeFSRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "mb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Briefly: the handle closes as the process finishes dying.
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
