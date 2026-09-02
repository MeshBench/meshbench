package session

import (
	"os"
	"testing"
)

// The per-user directories, pointed somewhere harmless for this whole package.
//
// Every verb the tree serves is registered here, and some of them write:
// run.save puts a record where the operator's own saved runs live. Tests that
// walk the verb list therefore added one to somebody's real history on every
// run - two hundred and fifty-one records had accumulated on the machine this
// was found on, and the panels that read them were failing against the pile.
//
// Done here rather than in each test because the ones that caused it are not
// the ones that look like they touch the disk: they call every verb by name
// and cannot know which of them writes. A test that wants its own directory
// still calls t.Setenv and still wins, because this only sets a default.
//
// Both spellings of each, because os.UserCacheDir and os.UserConfigDir read
// the XDG variables on Unix and %LocalAppData% / %AppData% on Windows, so
// setting only the XDG pair isolates Linux and leaves Windows writing into the
// real profile.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "meshbench-session")
	if err != nil {
		os.Exit(1)
	}
	for _, k := range []string{"XDG_CACHE_HOME", "LOCALAPPDATA", "XDG_CONFIG_HOME", "APPDATA"} {
		if err := os.Setenv(k, dir); err != nil {
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
