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

// consentSim is a session with somewhere to remember an answer and an empty
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
	t.Setenv("XDG_CONFIG_HOME", dir)
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

// A machine nobody has asked does not download, and says what it would cost.
func TestAFreshMachineIsAskedBeforeItsBandwidthIsSpent(t *testing.T) {
	s, st, ctx := consentSim(t)
	if s.terrainAllowed() {
		t.Fatal("a machine that has never been asked is already allowing downloads")
	}
	ts, ok := s.terrain().(*terrain.TileStore)
	if !ok {
		t.Fatal("no tile store")
	}
	if !ts.Offline {
		t.Error("the tile store would download without being allowed to")
	}
	if !s.heldForTerrain(ctx, st, twoDistantNodes()) {
		t.Fatal("the warm went ahead without asking")
	}
	said := strings.Join(st.Snapshot().Log, "\n")
	for _, want := range []string{"MB", "terrain tiles", "Nothing has been downloaded"} {
		if !strings.Contains(said, want) {
			t.Errorf("the held warm never said %q:\n%s", want, said)
		}
	}
	// And nothing is left to wait on: a run held behind a measurement nobody
	// is doing waits for ever.
	if s.warming() {
		t.Error("a held warm still reports itself as running")
	}
}

// Answered either way, it stops asking.
func TestAnAnsweredMachineIsNotAskedAgain(t *testing.T) {
	for _, on := range []bool{true, false} {
		s, st, ctx := consentSim(t)
		if _, err := st.Do(ctx, "terrain.allow", map[string]any{"on": on}); err != nil {
			t.Fatalf("terrain.allow: %v", err)
		}
		if s.heldForTerrain(ctx, st, twoDistantNodes()) {
			t.Errorf("on=%v: an answered machine was asked again", on)
		}
		if s.terrainAllowed() != on {
			t.Errorf("on=%v: the answer was not kept", on)
		}
		ts, _ := s.terrain().(*terrain.TileStore)
		if ts != nil && ts.Offline == on {
			t.Errorf("on=%v: the running store was not told", on)
		}
		// Kept for the next launch as well, not only for this session.
		b, err := os.ReadFile(s.prefsFile)
		if err != nil {
			t.Fatalf("settings: %v", err)
		}
		if !strings.Contains(string(b), "terrain_downloads") {
			t.Errorf("on=%v: the answer never reached the settings file: %s", on, b)
		}
	}
}

// A session with nowhere to keep an answer is a test or an embedding, and
// asking it a question it cannot remember answering would ask every launch.
func TestASessionWithNoSettingsFileIsNotAsked(t *testing.T) {
	st := state.New(10)
	s := &Sim{}
	Register(st, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)
	if !s.terrainAllowed() {
		t.Error("a session with no settings file refuses downloads it can never be granted")
	}
	if s.heldForTerrain(ctx, st, twoDistantNodes()) {
		t.Error("a session with no settings file held a warm to ask a question it cannot keep the answer to")
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

// The prefetch verb does not grant its own permission.
func TestPrefetchWillNotSpendWhatWasNotAllowed(t *testing.T) {
	_, st, ctx := consentSim(t)
	if _, err := st.Do(ctx, "project.open", "fem-e22"); err != nil {
		t.Fatalf("project.open: %v", err)
	}
	_, err := st.Do(ctx, "terrain.prefetch", nil)
	if err == nil || !strings.Contains(err.Error(), "terrain.allow") {
		t.Errorf("a prefetch with no permission gave %v, which does not name the way to grant it", err)
	}
}
