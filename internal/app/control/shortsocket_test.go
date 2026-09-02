package control

import (
	"os"
	"testing"
)

// shortSocketDir is a temporary directory with a short name, for a unix socket
// to live in.
//
// t.TempDir() spells the test's own name into the path, and a unix socket path
// is capped at 104 bytes - the limit this tree states in its own refusal,
// "choose a shorter one, or use tcp". So a test with a descriptive name cannot
// put a socket under its own temporary directory, and on Windows that is
// certain rather than likely: %LOCALAPPDATA%\Temp is already thirty-two of
// the hundred and four before a name is added.
//
// Measured there, a 51-byte path listens and a 119-byte one answers "bind:
// invalid argument" - which reads as a platform with no AF_UNIX and is nothing
// of the kind. Five tests were failing that way.
func shortSocketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "mb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
