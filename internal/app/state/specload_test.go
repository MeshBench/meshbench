package state_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func writeSpecFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const oneVerb = `{"a.verb":{"what":"do the thing","params":[` +
	`{"name":"who","type":"string","what":"which one"}]}}`

// The descriptions are split across files for the sake of whoever edits them,
// and merged for everything downstream, so nothing but the loader is in a
// position to notice two files claiming one verb. The store cannot: it never
// sees a description at all now.
func TestAVerbDescribedInTwoFilesIsRefused(t *testing.T) {
	root := t.TempDir()
	writeSpecFile(t, root, "first.verbs.json", oneVerb)
	writeSpecFile(t, filepath.Join(root, "deeper"), "second.verbs.json", oneVerb)

	_, err := state.LoadSpecs(root)
	if err == nil {
		t.Fatal("two files describing one verb loaded without complaint")
	}
	// Named, both of them: the point of refusing is that somebody has to go and
	// delete one, and a merge conflict resolved the wrong way leaves no other
	// clue where the second copy came from.
	for _, want := range []string{"a.verb", "first.verbs.json", "second.verbs.json"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %s: %v", want, err)
		}
	}
}

func TestSpecsMergeAcrossFiles(t *testing.T) {
	root := t.TempDir()
	writeSpecFile(t, root, "first.verbs.json", oneVerb)
	writeSpecFile(t, filepath.Join(root, "deeper"), "second.verbs.json",
		`{"b.verb":{"what":"do the other thing"}}`)
	writeSpecFile(t, root, "notes.json", "this is not a spec file and is not read")

	specs, err := state.LoadSpecs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 {
		t.Fatalf("loaded %d verbs, want 2", len(specs))
	}
	if got := specs["a.verb"].Params[0].Name; got != "who" {
		t.Errorf("the parameter came back as %q", got)
	}
}

// A misspelt key is worse than a missing one: it reads as written down and
// arrives nowhere.
func TestASpecFileWithAnUnknownFieldIsRefused(t *testing.T) {
	root := t.TempDir()
	writeSpecFile(t, root, "first.verbs.json",
		`{"a.verb":{"what":"do the thing","retruns":["x"]}}`)
	if _, err := state.LoadSpecs(root); err == nil {
		t.Fatal("an unknown field loaded without complaint")
	}
}

func TestAParameterTypeNoClientCanBindIsRefused(t *testing.T) {
	root := t.TempDir()
	writeSpecFile(t, root, "first.verbs.json",
		`{"a.verb":{"what":"do the thing","params":[`+
			`{"name":"who","type":"duration","what":"how long"}]}}`)
	_, err := state.LoadSpecs(root)
	if err == nil {
		t.Fatal("a parameter type nothing can bind loaded without complaint")
	}
	if !strings.Contains(err.Error(), "duration") {
		t.Errorf("the refusal does not name the type: %v", err)
	}
}
