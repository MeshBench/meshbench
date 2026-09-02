// What a study says about the ground it is standing on.
//
// The failure being pinned here is a raster that came back over an empty tile
// cache and looked exactly like a raster over real ridges: free space
// everywhere, every link closing, and a `started: true` for the script that
// asked. The picture was drawn, the job finished, and nothing anywhere said the
// hills were missing.
package study

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// groundStudy is the study verbs over a two-repeater network on a machine with
// an empty tile cache and somewhere to remember a terrain answer.
//
// answer is what the operator said, or nil for the fresh install nobody has
// asked. It goes in through the settings file rather than through terrain.allow
// because that is where a launch reads it from, and because the verb is not
// registered in this package.
func groundStudy(t *testing.T, answer *bool) (*state.Store, *session.Sim) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("APPDATA", home)
	t.Setenv("XDG_CACHE_HOME", home)
	t.Setenv("LOCALAPPDATA", home)
	t.Setenv("HOME", home)
	if answer != nil {
		dir := filepath.Join(home, "meshbench")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(map[string]any{"terrain_downloads": *answer})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "workbench2.json"), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	s := &session.Sim{}
	if err := s.LoadPrefs(); err != nil {
		t.Fatalf("prefs: %v", err)
	}
	nodes := []scenario.Node{
		nodeAt("West Lomond", scenario.SimpleRepeater, 56.25, -3.29),
		nodeAt("Dunfermline", scenario.SimpleRepeater, 56.07, -3.46),
	}
	s.BuildSeeded(nodes, 869.525, 1)
	t.Cleanup(func() {
		if e := s.Engine(); e != nil {
			_ = e.Close()
		}
	})

	st := state.New(10)
	registerCoverageVerbs(st, s)
	registerCoverageMap(st, s)
	registerPlanningVerbs(st, s)
	registerCoverageCombined(st, s)
	st.Handle("test.nodes", func(w *state.World, p any) (any, error) {
		w.Nodes, _ = p.([]state.Node)
		return nil, nil
	})
	go st.Run(t.Context())
	if _, err := st.Do(t.Context(), "test.nodes", session.StateNodes(nodes)); err != nil {
		t.Fatal(err)
	}
	return st, s
}

// A raster over ground nobody was asked about is refused, because a plausible
// picture is worse than a question.
func TestARasterOverUnchosenBareEarthIsRefused(t *testing.T) {
	st, _ := groundStudy(t, nil)
	for _, verb := range []string{"coverage.map", "coverage.start"} {
		got, err := st.Do(t.Context(), verb, nil)
		if err == nil {
			t.Fatalf("%s answered over an empty tile cache: %v", verb, got)
		}
		for _, want := range []string{"free space", "terrain.allow"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s refused without saying %q: %v", verb, want, err)
			}
		}
	}
}

// A raster the operator chose to run offline still runs, and carries what it
// ran over in its own result - not only in a log line, because studies are run
// by scripts that never read one.
func TestAChosenOfflineRasterAnswersAndCarriesItsGround(t *testing.T) {
	off := false
	st, _ := groundStudy(t, &off)
	got, err := st.Do(t.Context(), "coverage.map", nil)
	if err != nil {
		t.Fatalf("a deliberate offline raster was refused: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("coverage.map answered %T", got)
	}
	g, ok := m["ground"].(map[string]any)
	if !ok {
		t.Fatalf("coverage.map's result carries no ground: %v", m)
	}
	if g["state"] != state.GroundBare {
		t.Errorf("a raster over an empty cache reports ground %v", g)
	}
	if g["chosen"] != true {
		t.Errorf("a run the operator asked for reads as a silent fallback: %v", g)
	}
	note, _ := g["note"].(string)
	if !strings.Contains(note, "free space") {
		t.Errorf("the ground note does not name what bare earth means: %q", note)
	}
	// And in the interface too: the chrome's caveat line reads this.
	if snap := st.Snapshot(); !snap.Ground.Bare() {
		t.Errorf("the world did not learn what the study stood on: %+v", snap.Ground)
	}
}
