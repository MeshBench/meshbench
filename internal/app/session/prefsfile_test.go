package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// The settings survive the round trip, and the write leaves nothing else
// behind: a temporary file still sitting in ~/.config is litter that grows.
func TestPrefsRoundTripAndLeaveNoLitter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meshbench", "workbench2.json")
	on := true
	want := Prefs{TileCacheGB: 4, GPU: &on, RFMode: "waveform", CoverageCells: 512}
	if err := writePrefs(path, want); err != nil {
		t.Fatalf("writePrefs: %v", err)
	}
	got, err := readPrefs(path)
	if err != nil || got == nil {
		t.Fatalf("readPrefs: %v, %v", got, err)
	}
	if got.TileCacheGB != 4 || got.RFMode != "waveform" || got.CoverageCells != 512 ||
		got.GPU == nil || !*got.GPU {
		t.Errorf("came back as %+v, want %+v", *got, want)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("the settings directory holds %d files, want only the settings", len(entries))
	}
}

// No file at all is not a failure: nothing has been chosen yet.
func TestNoSettingsFileIsNotAProblem(t *testing.T) {
	p, err := readPrefs(filepath.Join(t.TempDir(), "never-written.json"))
	if p != nil || err != nil {
		t.Errorf("got %v, %v; want nothing and no error", p, err)
	}
}

// A settings file that cannot be parsed is reported. Silence here reverted
// every remembered choice and said nothing about it.
func TestATruncatedSettingsFileIsReported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workbench2.json")
	whole, err := json.MarshalIndent(Prefs{TileCacheGB: 4, RFMode: "waveform"}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, whole[:len(whole)/2], 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := readPrefs(path); err == nil {
		t.Fatal("half a settings file read as settings")
	}

	s := &Sim{prefsFile: path}
	err = s.LoadPrefs()
	if err == nil {
		t.Fatal("LoadPrefs said nothing about an unreadable file")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("the complaint does not name the file: %v", err)
	}
	if s.rfMode == "waveform" {
		t.Error("half a file was applied")
	}
}

// An interrupted write cannot leave a file that loads as valid-but-partial:
// the target is only ever replaced by a complete file, so what a reader sees
// is the old settings or the new ones.
func TestAnInterruptedWriteLeavesTheOldSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workbench2.json")
	if err := writePrefs(path, Prefs{TileCacheGB: 4, Basemap: "carto-dark"}); err != nil {
		t.Fatal(err)
	}

	// A write that fails at the last moment: the directory is read-only, so
	// the rename cannot happen. Every earlier step has already run.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Skip("cannot make the directory read-only here:", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if err := writePrefs(path, Prefs{TileCacheGB: 9, Basemap: "esri-topo"}); err == nil {
		t.Skip("the write succeeded despite a read-only directory; not root-safe")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	got, err := readPrefs(path)
	if err != nil || got == nil {
		t.Fatalf("the settings no longer load: %v, %v", got, err)
	}
	if got.TileCacheGB != 4 || got.Basemap != "carto-dark" {
		t.Errorf("a failed write changed the file: %+v", *got)
	}
}

// A failed save is said out loud, and the verb that promised the next launch
// does not promise it.
func TestAFailedSaveIsReported(t *testing.T) {
	// The settings file is put behind a path that cannot be written on any
	// platform: its parent is a regular file rather than a directory. A
	// read-only directory was used here before, which Unix enforces and
	// Windows does not - so the save succeeded there, the test failed, and it
	// looked like the reporting was broken rather than the setup.
	dir := t.TempDir()
	blocked := filepath.Join(dir, "notadirectory")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	st := state.New(10)
	s := &Sim{persist: true, prefsFile: filepath.Join(blocked, "workbench2.json")}
	Register(st, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	if _, err := st.Do(ctx, "map.basemap", map[string]any{"id": "carto-light"}); err != nil {
		t.Fatalf("map.basemap: %v", err)
	}
	said := strings.Join(st.Snapshot().Log, "\n")
	if !strings.Contains(said, "settings not saved") {
		t.Errorf("nothing said the settings could not be written:\n%s", said)
	}
	if strings.Contains(said, "next launch") {
		t.Errorf("a save that failed still promised the next launch:\n%s", said)
	}
}
