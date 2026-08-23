package session

import (
	"bufio"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var updateManifest = flag.Bool("update-manifest", false,
	"rewrite docs/verbs.json and docs/verbs-undescribed.txt from the tree")

// manifest is the committed shape of docs/verbs.json.
type manifest struct {
	// Verbs, by name, so a diff of this file reads as a diff of what changed
	// rather than of where things moved to.
	Verbs map[string]any `json:"verbs"`
}

// The manifest is generated from the registration, and CI holds it current.
//
//	go test ./internal/app/session -run TestTheVerbManifest -update-manifest
//
// Committed rather than generated at build time because it is read by things
// that are not this repository - the clients, the published reference, and
// eventually MeshCore's own CI - and a file they can fetch is worth more than
// a program they must run.
func TestTheVerbManifestIsCurrent(t *testing.T) {
	st, _ := Boot(Options{NoPrefs: true, Headless: true})

	specs := st.Specs()
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
	if string(got) != string(want) {
		t.Errorf("docs/verbs.json is out of date; run:\n"+
			"  go test ./internal/app/session -run TestTheVerbManifest -update-manifest\n"+
			"it holds %d bytes and the tree describes %d verbs", len(got), len(specs))
	}
}

// Every registered verb describes itself.
//
// A ratchet rather than a rule, for now: 226 verbs were registered before any
// of them could say what they took, so the ones that still cannot are listed in
// docs/verbs-undescribed.txt and that list may only shrink. A verb added today
// has nowhere to hide - it is not on the list, so it must describe itself.
func TestEveryVerbDescribesItself(t *testing.T) {
	st, _ := Boot(Options{NoPrefs: true, Headless: true})

	// The other direction - a description naming no verb - cannot happen here,
	// because HandleSpec registers both at once. Where it can happen is on the
	// surfaces built over the verbs, and that is what
	// TestEveryToolNamesARegisteredVerb walks in internal/app/mcp.
	allowed := readUndescribed(t)
	var unexpected []string
	for _, v := range st.Undescribed() {
		if !allowed[v] {
			unexpected = append(unexpected, v)
		}
		delete(allowed, v)
	}
	if len(unexpected) > 0 {
		sort.Strings(unexpected)
		t.Errorf("%d verbs say nothing about themselves and are not on the list:\n  %s\n"+
			"describe them at their st.HandleSpec call",
			len(unexpected), strings.Join(unexpected, "\n  "))
	}
	// The ratchet: a verb that has learned to describe itself, or been
	// deleted, must leave the list, or the list stops meaning anything.
	if len(allowed) > 0 {
		var stale []string
		for v := range allowed {
			stale = append(stale, v)
		}
		sort.Strings(stale)
		t.Errorf("%d verbs on docs/verbs-undescribed.txt now describe themselves"+
			" or no longer exist; remove them:\n  %s",
			len(stale), strings.Join(stale, "\n  "))
	}
	t.Logf("%d verbs described, %d still to go", len(st.Specs()), len(st.Undescribed()))
}

func readUndescribed(t *testing.T) map[string]bool {
	t.Helper()
	path := filepath.Join("..", "..", "..", "docs", "verbs-undescribed.txt")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	out := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[line] = true
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
