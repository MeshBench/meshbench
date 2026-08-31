package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
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

// A matrix saved under a fingerprint comes back exactly, and an unknown
// fingerprint answers nil rather than somebody else's matrix.
func TestMatrixRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := map[[2]int]float64{{0, 1}: 121.5, {0, 2}: 140.25, {5, 9}: 99.75}
	saveMatrix(dir, 0xdeadbeef, m)
	got := loadMatrix(dir, 0xdeadbeef)
	if len(got) != len(m) {
		t.Fatalf("loaded %d pairs, want %d", len(got), len(m))
	}
	for k, v := range m {
		if got[k] != v {
			t.Errorf("pair %v came back %v, want %v", k, got[k], v)
		}
	}
	if loadMatrix(dir, 0x1234) != nil {
		t.Error("an unknown fingerprint answered a matrix")
	}
	// Persistence off means no directory and a quiet no-op.
	s := &Sim{}
	if s.matrixDir() != "" {
		t.Error("a session without LoadPrefs got a matrix directory")
	}
}

// The basemap choice lands in the session and comes back, which is what the
// command reads at the next launch.
func TestBasemapChoiceIsKept(t *testing.T) {
	st := state.New(10)
	s := &Sim{}
	Register(st, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)
	if _, err := st.Do(ctx, "map.basemap", map[string]any{"id": "carto-light"}); err != nil {
		t.Fatalf("map.basemap: %v", err)
	}
	if got := s.Basemap(); got != "carto-light" {
		t.Errorf("the session remembers %q, want carto-light", got)
	}
}

// A session that never called LoadPrefs never writes a file, which is what
// keeps every other test from scribbling on the developer's own settings.
func TestSavePrefsIsOffByDefault(t *testing.T) {
	s := &Sim{}
	s.prefs.TileCacheGB = 5
	// A no-op, and a path it would certainly fail to write to if it were not.
	s.prefsFile = filepath.Join(t.TempDir(), "no", "such", "dir", "prefs.json")
	if err := s.savePrefs(nil); err != nil {
		t.Errorf("a session with persistence off tried to write: %v", err)
	}
	if s.persist {
		t.Error("persist turned itself on")
	}
	if _, err := os.Stat(s.prefsFile); !os.IsNotExist(err) {
		t.Error("a session with persistence off wrote a settings file")
	}
}
