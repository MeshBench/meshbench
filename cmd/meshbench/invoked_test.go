package main

import (
	"os"
	"testing"
)

// The release ships this binary as "meshbench" while the package is
// cmd/meshbench, so a hardcoded name is wrong for one audience or the other.
func TestInvokedIsWhatTheUserTyped(t *testing.T) {
	saved := os.Args
	defer func() { os.Args = saved }()

	for _, c := range []struct{ argv0, want string }{
		{"meshbench", "meshbench"},
		{"/usr/local/bin/meshbench", "meshbench"},
		{"./meshbench", "meshbench"},
		// filepath.Base is platform-specific, so a Windows path cannot be
		// exercised from here - on Windows it splits on the backslash and the
		// suffix trim does the rest, which is what the .exe case below covers.
		{"meshbench.exe", "meshbench"},
		{"", "meshbench"},
	} {
		os.Args = []string{c.argv0}
		if got := invoked(); got != c.want {
			t.Errorf("invoked() with argv[0]=%q\n  got  %q\n  want %q", c.argv0, got, c.want)
		}
	}

	os.Args = nil
	if got := invoked(); got != "meshbench" {
		t.Errorf("invoked() with no argv at all: got %q", got)
	}
}
