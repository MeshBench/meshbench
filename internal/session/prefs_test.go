package session

import (
	"os"
	"path/filepath"
	"testing"
)

// A move carries every file over, keeps the layout, and empties the source.
func TestMoveTreeCarriesEverything(t *testing.T) {
	oldDir, newDir := t.TempDir(), filepath.Join(t.TempDir(), "cache")
	files := []string{"12/2048/1361.png", "12/2048/1362.png", "12/2049/1361.png"}
	for _, f := range files {
		p := filepath.Join(oldDir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatal(err)
	}

	var last, total int
	n, err := moveTree(oldDir, newDir, func(d, of int) { last, total = d, of })
	if err != nil {
		t.Fatalf("moveTree: %v", err)
	}
	if n != len(files) {
		t.Errorf("moved %d files, want %d", n, len(files))
	}
	if last != len(files) || total != len(files) {
		t.Errorf("progress ended at %d/%d, want %d/%d", last, total, len(files), len(files))
	}
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(newDir, f))
		if err != nil {
			t.Fatalf("%s did not arrive: %v", f, err)
		}
		if string(b) != f {
			t.Errorf("%s arrived with the wrong content", f)
		}
		if _, err := os.Stat(filepath.Join(oldDir, f)); !os.IsNotExist(err) {
			t.Errorf("%s was left behind in the old cache", f)
		}
	}
}

// Moving out of a directory that does not exist yet is not an error: there is
// simply nothing to carry.
func TestMoveTreeFromNowhere(t *testing.T) {
	n, err := moveTree(filepath.Join(t.TempDir(), "never"), t.TempDir(), nil)
	if err != nil || n != 0 {
		t.Errorf("got %d, %v; want 0 files and no error", n, err)
	}
}

// The moves that cannot work are refused before any file moves.
func TestValidateCacheDirRefusesNesting(t *testing.T) {
	oldDir := t.TempDir()
	for _, bad := range []string{
		oldDir,
		filepath.Join(oldDir, "sub"),
		filepath.Dir(oldDir),
	} {
		if _, err := validateCacheDir(oldDir, bad); err == nil {
			t.Errorf("validateCacheDir(%q, %q) allowed a nested move", oldDir, bad)
		}
	}
	good := filepath.Join(t.TempDir(), "fresh")
	abs, err := validateCacheDir(oldDir, good)
	if err != nil {
		t.Fatalf("a sibling directory was refused: %v", err)
	}
	if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
		t.Errorf("validateCacheDir did not create %s", abs)
	}
}

// A session that never called LoadPrefs never writes a file, which is what
// keeps every other test from scribbling on the developer's own settings.
func TestSavePrefsIsOffByDefault(t *testing.T) {
	s := &Sim{}
	s.prefs.TileCacheGB = 5
	s.savePrefs() // must be a no-op; nothing to assert beyond not crashing
	if s.persist {
		t.Error("persist turned itself on")
	}
}
