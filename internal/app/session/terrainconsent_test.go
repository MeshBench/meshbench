package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// consentSim is a session with somewhere to remember the setting and an empty
// tile cache, which is the state a fresh install is in.
func consentSim(t *testing.T) (*Sim, *state.Store, context.Context) {
	t.Helper()
	dir := t.TempDir()
	// A session that persists writes its measured matrix to the machine's own
	// cache directory, and reads it back on the next launch for the same
	// geometry. Left pointed at the developer's home that is a warm in one test
	// priming another - which shows up as a held warm that mysteriously is not
	// held, because the matrix it would have measured was already on disk.
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPDATA", dir)
	t.Setenv("HOME", dir)
	s := &Sim{
		persist:   true,
		prefsFile: filepath.Join(dir, "workbench2.json"),
	}
	s.prefs.TileCacheDir = filepath.Join(dir, "tiles")
	st := state.New(10)
	Register(st, s)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go st.Run(ctx)
	return s, st, ctx
}

func twoDistantNodes() []scenario.Node {
	return []scenario.Node{
		{Name: "a", Position: scenario.LatLon{Lat: 56.0, Lon: -4.0},
			HeightAGLm: 10, TxPowerDBm: 22},
		{Name: "b", Position: scenario.LatLon{Lat: 56.2, Lon: -4.3},
			HeightAGLm: 10, TxPowerDBm: 22},
	}
}

// A fresh install downloads, because a flat earth is not a safe default.
//
// This used to hold the warm and ask. The question made bare earth the resting
// state of every install nobody had answered, which is the most optimistic
// model there is and the one the rest of this file exists to keep honest.
func TestAFreshMachineDownloadsWithoutBeingAsked(t *testing.T) {
	s, _, _ := consentSim(t)
	if !s.terrainAllowed() {
		t.Fatal("a machine nobody has configured refuses to fetch the ground")
	}
	ts, ok := s.terrain().(*terrain.TileStore)
	if !ok {
		t.Fatal("no tile store")
	}
	if ts.Offline {
		t.Error("the tile store will not download on a fresh install")
	}
}

// Turned off, it stays off, here and on the next launch.
func TestTheSwitchIsKeptAndReachesTheRunningStore(t *testing.T) {
	for _, on := range []bool{true, false} {
		s, st, ctx := consentSim(t)
		if _, err := st.Do(ctx, "terrain.allow", map[string]any{"on": on}); err != nil {
			t.Fatalf("terrain.allow: %v", err)
		}
		if s.terrainAllowed() != on {
			t.Errorf("on=%v: the setting was not kept", on)
		}
		ts, _ := s.terrain().(*terrain.TileStore)
		if ts != nil && ts.Offline == on {
			t.Errorf("on=%v: the running store was not told", on)
		}
		b, err := os.ReadFile(s.prefsFile)
		if err != nil {
			t.Fatalf("settings: %v", err)
		}
		if !strings.Contains(string(b), "terrain_downloads") {
			t.Errorf("on=%v: the setting never reached the settings file: %s", on, b)
		}
	}
}

// A session with nowhere to keep a setting is a test or an embedding, and it
// downloads: there is nothing to read a refusal out of.
func TestASessionWithNoSettingsFileDownloads(t *testing.T) {
	st := state.New(10)
	s := &Sim{}
	Register(st, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)
	_ = ctx
	if !s.terrainAllowed() {
		t.Error("a session with no settings file refuses downloads nothing can grant")
	}
}

// The download's own words say what is being downloaded and what it has cost.
func TestTheTerrainLineNamesTheDownloadAndItsSize(t *testing.T) {
	got := terrainWords(84<<20, 366<<20)
	for _, want := range []string{"terrain", "84 MB", "about 366 MB"} {
		if !strings.Contains(got, want) {
			t.Errorf("the terrain line %q does not say %q", got, want)
		}
	}
	if strings.Contains(terrainWords(1<<20, 0), "about") {
		t.Error("a line with no estimate still quotes one")
	}
	// An estimate the download has already passed is not a total.
	if got := terrainWords(390<<20, 365<<20); strings.Contains(got, "of about") {
		t.Errorf("a download past its estimate reports a fraction over one: %q", got)
	}
}

// The quote somebody decides on must not flatter.
//
// 6,233 tiles of Scotland and Ireland came to 525 MB, an average of 82 kB.
// The figure was 60 kB, so the download somebody was asked to agree to was
// quoted at 365 MB and cost half again as much.
func TestTheDownloadEstimateDoesNotUndersell(t *testing.T) {
	const measured = 524760837 / 6233
	ts := &terrain.TileStore{CacheDir: t.TempDir()}
	got := ts.EstimateTiles(make([][2]int, 6233))
	if per := got.BytesRough / 6233; per < measured*9/10 {
		t.Errorf("a tile is priced at %d bytes against %d measured, so the quote "+
			"is %d MB for a download of %d MB",
			per, measured, got.BytesRough>>20, int64(measured)*6233>>20)
	}
}

// The switch is honoured by the prefetch too: off means off, and the refusal
// names the way back.
func TestPrefetchWillNotSpendWhatWasSwitchedOff(t *testing.T) {
	_, st, ctx := consentSim(t)
	if _, err := st.Do(ctx, "terrain.allow", map[string]any{"on": false}); err != nil {
		t.Fatalf("terrain.allow: %v", err)
	}
	if _, err := st.Do(ctx, "project.open", "fem-e22"); err != nil {
		t.Fatalf("project.open: %v", err)
	}
	_, err := st.Do(ctx, "terrain.prefetch", nil)
	if err == nil || !strings.Contains(err.Error(), "terrain.allow") {
		t.Errorf("a prefetch with downloads off gave %v, which does not name the way back", err)
	}
}
