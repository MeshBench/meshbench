package session

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

var updateManifest = flag.Bool("update-manifest", false,
	"rewrite docs/verbs.json from the .verbs.json files beside the code")

// manifest is the committed shape of docs/verbs.json.
type manifest struct {
	// Verbs, by name, so a diff of this file reads as a diff of what changed
	// rather than of where things moved to.
	Verbs map[string]any `json:"verbs"`
}

// describedVerbs is every sibling .verbs.json in the tree, merged.
func describedVerbs(t *testing.T) map[string]state.Spec {
	t.Helper()
	specs, err := state.LoadSpecs(filepath.Join("..", "..", "..", "internal"))
	if err != nil {
		t.Fatal(err)
	}
	return specs
}

// The manifest is generated from the descriptions, and CI holds it current.
//
//	go test ./internal/app/session -run TestTheVerbManifest -update-manifest
//
// Committed rather than generated at build time because it is read by things
// that are not this repository - the clients, the published reference, and
// eventually MeshCore's own CI - and a file they can fetch is worth more than
// a program they must run, or than eighty files they must merge themselves.
func TestTheVerbManifestIsCurrent(t *testing.T) {
	specs := describedVerbs(t)
	verbs := make(map[string]any, len(specs))
	for name, sp := range specs {
		verbs[name] = sp
	}
	want, err := json.MarshalIndent(manifest{Verbs: verbs}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')

	path := filepath.Join("..", "..", "..", "docs", "verbs.json")
	if *updateManifest {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s: %d verbs described", path, len(specs))
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v\nrun: go test ./internal/app/session -run TestTheVerbManifest -update-manifest", err)
	}
	// Compared with line endings normalised. The manifest is generated with
	// LF and a checkout on Windows holds CRLF, so a byte comparison is red on
	// every branch there - and a check that is always red is one nobody reads
	// when it means something.
	if strings.ReplaceAll(string(got), "\r\n", "\n") != string(want) {
		t.Errorf("docs/verbs.json is out of date; run:\n"+
			"  go test ./internal/app/session -run TestTheVerbManifest -update-manifest\n"+
			"it holds %d bytes and the tree describes %d verbs", len(got), len(specs))
	}
}

// The descriptions and the store are held against each other, in the one
// direction where a mismatch is a mistake rather than work not done yet.
//
// A description naming a verb nothing registers is a rename that left its old
// documentation behind, and the reference would print an entry for a call that
// is refused, so it fails here. The other way round is not a failure: the
// generated reference marks an undescribed verb in place rather than dropping
// it, so the gap is visible to a reader without also stopping the build.
func TestEveryDescriptionNamesAVerbTheStoreRegisters(t *testing.T) {
	st, _ := Boot(Options{NoPrefs: true, Headless: true})

	registered := map[string]bool{}
	for _, v := range st.Verbs() {
		registered[v] = true
	}
	specs := describedVerbs(t)

	var stale, undescribed []string
	for name := range specs {
		if !registered[name] {
			stale = append(stale, name)
		}
	}
	for name := range registered {
		if _, ok := specs[name]; !ok {
			undescribed = append(undescribed, name)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("%d verbs are described but the store does not register them; "+
			"delete their entries or fix the name:\n  %s",
			len(stale), strings.Join(stale, "\n  "))
	}
	sort.Strings(undescribed)
	t.Logf("%d verbs described, %d still to go", len(specs), len(undescribed))
}
