package study

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/fixture"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// listed reads project.list's reply on a store whose configuration directory
// is a temporary one, which is the whole point: every developer machine has
// projects saved, and a list that is only ever exercised there is a list
// nobody has seen empty.
func listed(t *testing.T, st *state.Store) (projects, fixtures []string, dir string) {
	t.Helper()
	res, err := st.Do(t.Context(), "project.list", nil)
	if err != nil {
		t.Fatalf("project.list: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("project.list answered %T, not a map", res)
	}
	projects, _ = m["projects"].([]string)
	fixtures, _ = m["fixtures"].([]string)
	dir, _ = m["dir"].(string)
	return projects, fixtures, dir
}

// A machine that has never run MeshBench has saved no networks, and the first
// instruction anybody is given is to open a shipped one. So the reply that
// fills that chooser has to name them, and every name it offers has to be one
// project.open can actually resolve.
func TestProjectListOffersTheShippedNetworksOnAFreshInstall(t *testing.T) {
	st, _ := aStudy(t)
	projects, fixtures, dir := listed(t, st)

	if len(projects) != 0 {
		t.Fatalf("a fresh install has saved nothing, but project.list named %v", projects)
	}
	if dir == "" {
		t.Error("project.list said no projects directory, so a caller cannot build a path to one")
	}
	if !slices.Contains(fixtures, "fixture-fife-strict") {
		t.Fatalf("project.list offered %v, which is not the shipped network the "+
			"first-simulation page tells a new user to open", fixtures)
	}
	for _, name := range fixtures {
		if _, _, err := fixture.Find(name); err != nil {
			t.Errorf("project.list offered %q, which does not resolve: %v", name, err)
		}
	}
}

// And the user's own saved networks are still there, under the directory the
// reply names, told apart from the shipped ones by being in a different list
// rather than by being copied in among them.
func TestProjectListKeepsSavedNetworksApartFromShippedOnes(t *testing.T) {
	st, _ := aStudy(t)
	dir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "meshbench", "projects")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "my-glen.json"), []byte(`{"nodes":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	projects, fixtures, gotDir := listed(t, st)
	if !slices.Contains(projects, "my-glen") {
		t.Errorf("a saved network is missing from %v", projects)
	}
	if slices.Contains(fixtures, "my-glen") {
		t.Errorf("a saved network was offered as a shipped one: %v", fixtures)
	}
	if gotDir != dir {
		t.Errorf("project.list reads %q but says %q", dir, gotDir)
	}
}
