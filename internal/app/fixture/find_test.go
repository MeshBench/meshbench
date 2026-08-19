package fixture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The property 0.0.1 shipped without: an installed MeshBench can open its own
// example network from a working directory that has no fixtures in it, which
// is every installed layout - a .deb launched from $HOME, a macOS bundle
// launched from /, a Windows shortcut with its own start directory.
func TestAFixtureOpensWithNothingOnDisk(t *testing.T) {
	t.Chdir(t.TempDir())

	for _, name := range []string{"fife-strict", "fixture-fife-strict", "fife-strict.json"} {
		f, err := Load(name)
		if err != nil {
			t.Fatalf("Load(%q) from an empty directory: %v", name, err)
		}
		if len(f.Nodes) == 0 {
			t.Fatalf("Load(%q) returned a fixture with no nodes", name)
		}
	}
}

// A path still wins, and still means that file - somebody who typed a path
// meant it, even if a built-in shares the name.
func TestAPathBeatsTheBuiltInOfTheSameName(t *testing.T) {
	dir := t.TempDir()
	mine := filepath.Join(dir, "fife-strict.json")
	// One node, so it is distinguishable from the shipped fixture.
	body := `{"nodes":[{"name":"only-me","kind":"simple-repeater",
	          "position":{"lat":56.1,"lon":-3.2}}]}`
	if err := os.WriteFile(mine, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(mine)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Nodes) != 1 || f.Nodes[0].Name != "only-me" {
		t.Fatalf("a path did not win: got %d nodes", len(f.Nodes))
	}
}

// A fixture beside the binary is found without naming a path, which is what
// the tarball and the Windows zip rely on.
func TestAFixtureBesideTheBinaryIsFound(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip("no executable path here")
	}
	dir := filepath.Join(filepath.Dir(exe), "fixtures")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Skip("cannot write beside the test binary:", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	body := `{"nodes":[{"name":"beside-the-binary","kind":"simple-repeater",
	          "position":{"lat":56.1,"lon":-3.2}}]}`
	if err := os.WriteFile(filepath.Join(dir, "fixture-besider.json"), []byte(body), 0o644); err != nil {
		t.Skip("cannot write beside the test binary:", err)
	}
	t.Chdir(t.TempDir())
	f, err := Load("besider")
	if err != nil {
		t.Fatalf("a fixture beside the binary was not found: %v", err)
	}
	if f.Nodes[0].Name != "beside-the-binary" {
		t.Fatalf("found the wrong fixture: %s", f.Nodes[0].Name)
	}
}

// An unknown name says what the names are. The old failure was an
// os.ReadFile error naming a path the user never typed.
func TestAnUnknownNameListsTheOnesThatExist(t *testing.T) {
	_, err := Load("no-such-network")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "fife-strict") {
		t.Fatalf("the error does not say what is available: %v", err)
	}
}

func TestEveryShippedFixtureIsEmbedded(t *testing.T) {
	// The on-disk fixtures and the embedded ones are the same set; a fixture
	// added to the directory and not to the binary is one that works in
	// development and not in an install.
	onDisk, err := filepath.Glob("../../../fixtures/fixture-*.json")
	if err != nil || len(onDisk) == 0 {
		t.Skip("not running from the repository")
	}
	have := map[string]bool{}
	for _, n := range Embedded() {
		have[n] = true
	}
	for _, p := range onDisk {
		name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(p), "fixture-"), ".json")
		if !have[name] {
			t.Errorf("%s is on disk but not embedded", name)
		}
	}
}
