// Test isolation of the per-user directories, enforced.
//
// A test that wants its own cache writes t.Setenv("XDG_CACHE_HOME", dir) and
// then reads os.UserCacheDir(). That works on Linux and nowhere else:
// os.UserCacheDir reads XDG_CACHE_HOME on Unix, %LocalAppData% on Windows and
// $HOME/Library/Caches on macOS. So on Windows the override does nothing, the
// test writes into the real cache under the user's profile, and reads back
// whatever is already there.
//
// That is not a hypothetical. Found on a Windows machine where four tests
// failed at once - "read 200 runs, want 4", "found 251 runs, want 240", "the
// fabricated library reads as 12 builds, want 20" - because they were reading
// a real profile, and where the suite had by then planted a hundred run
// records into it. A test that writes into the user's own data is worse than a
// test that fails.
//
// The same applies to os.UserConfigDir and %AppData%, which is how a test for
// terrain consent came to be answered by whatever the person running it had
// already chosen.
//
// A tree-wide check rather than a helper package, because internal/ is nine
// layers and a tenth for one testing utility is not worth the exception: this
// fails on the machine of whoever writes the sixteenth one, while they still
// remember why.
package internal_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// paired is the variable each XDG one has to be set beside, so that whatever
// os.UserCacheDir and os.UserConfigDir actually read is pointed somewhere
// harmless on every platform.
var paired = map[string]string{
	"XDG_CACHE_HOME":  "LOCALAPPDATA",
	"XDG_CONFIG_HOME": "APPDATA",
}

// readsXDGItself are the packages whose code under test reads the XDG variable
// directly rather than through os.UserCacheDir, so the Windows equivalent
// would mean nothing to them. internal/ui/desktop looks up a cursor theme the
// way the freedesktop specification says to.
var readsXDGItself = []string{"ui/desktop"}

func TestTestsIsolateThePerUserDirectories(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(path)
		if rel == "cachehome_test.go" {
			return nil // the file that states the rule names both sides of it
		}
		for _, skip := range readsXDGItself {
			if strings.HasPrefix(rel, skip+"/") {
				return nil
			}
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		src := string(b)
		for xdg, native := range paired {
			// Counted rather than matched line by line: one file may set it
			// in several helpers, and every one of them needs the pair.
			if n := strings.Count(src, `"`+xdg+`"`); n > strings.Count(src, `"`+native+`"`) {
				t.Errorf("%s sets %s %d time(s) and %s fewer times.\n"+
					"os.UserCacheDir and os.UserConfigDir do not read the XDG "+
					"variables on Windows, so this test would read and write "+
					"the real profile there. Set both, off one t.TempDir().",
					rel, xdg, n, native)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
