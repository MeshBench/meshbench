// The layout map, enforced.
//
// CLAUDE.md carries a table of every package under internal/ and what it holds,
// and states the rule in as many words: "A new package updates it in the same
// commit - the map being wrong is worse than the map being short." That rule
// was broken the week it was written, by a package that shipped without its
// line, and nobody noticed until somebody diffed the two by hand.
//
// A map is not read looking for the road that is missing. So this reads it
// instead, in both directions: a package with no entry fails, and an entry
// naming a package that has gone fails too - the second being the one a
// deletion or a rename leaves behind.
//
// Deliberately a test rather than a CI step. A CI step checks the tree that CI
// happens to run on; a test fails on the machine of whoever moved the package,
// while they still remember why.
package internal_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// mapEntry matches a line of the table: two spaces, the package name, a
// trailing slash, then whitespace and its description. The layer headings
// themselves start at column zero and so do not match.
var mapEntry = regexp.MustCompile(`(?m)^ {2}([a-z][a-z0-9]*)/\s+\S`)

// layerEntry matches a layer's own heading, which sits at column zero as
// "internal/rf/" rather than being indented like the packages beneath it.
var layerEntry = regexp.MustCompile(`(?m)^internal/([a-z][a-z0-9]*)/`)

// inlineEntry catches the one layer written as a run of names on a single line
// rather than as its own block - the ui layer's toolkit packages.
var inlineEntry = regexp.MustCompile(`\b([a-z][a-z0-9]*)/`)

func TestLayoutMapMatchesTheTree(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "CLAUDE.md"))
	if err != nil {
		t.Fatalf("reading the layout map: %v", err)
	}
	// Only the fenced block is the map. Prose elsewhere names packages too,
	// and a mention in a sentence is not an entry in a table.
	block := layoutBlock(string(src))
	if block == "" {
		t.Fatal("no fenced layout block in CLAUDE.md: the map has been removed or its fence changed")
	}

	mapped := map[string]bool{}
	for _, m := range mapEntry.FindAllStringSubmatch(block, -1) {
		mapped[m[1]] = true
	}
	for _, m := range layerEntry.FindAllStringSubmatch(block, -1) {
		mapped[m[1]] = true
	}
	for _, line := range strings.Split(block, "\n") {
		// A line of several names and no description is the inline form.
		if strings.Count(line, "/") > 1 && !strings.Contains(strings.TrimSpace(line), "  ") {
			for _, m := range inlineEntry.FindAllStringSubmatch(line, -1) {
				mapped[m[1]] = true
			}
		}
	}

	found := map[string]bool{}
	layers, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading internal/: %v", err)
	}
	for _, layer := range layers {
		if !layer.IsDir() {
			continue
		}
		pkgs, err := os.ReadDir(layer.Name())
		if err != nil {
			t.Fatalf("reading internal/%s: %v", layer.Name(), err)
		}
		for _, pkg := range pkgs {
			if !pkg.IsDir() || strings.HasPrefix(pkg.Name(), "testdata") {
				continue
			}
			found[pkg.Name()] = true
			if !mapped[pkg.Name()] {
				t.Errorf("internal/%s/%s has no entry in CLAUDE.md's layout map.\n"+
					"A new package updates the map in the same commit - the map being "+
					"wrong is worse than the map being short.", layer.Name(), pkg.Name())
			}
		}
		if !mapped[layer.Name()] {
			t.Errorf("the layer internal/%s has no entry in CLAUDE.md's layout map", layer.Name())
		}
		found[layer.Name()] = true
	}

	for name := range mapped {
		if !found[name] {
			t.Errorf("CLAUDE.md's layout map names %q, which is not a directory under internal/.\n"+
				"A rename or a deletion left the map pointing at nothing.", name)
		}
	}
}

// layoutBlock returns the fenced code block that holds the map, found by the
// layer it must start with rather than by counting fences.
func layoutBlock(s string) string {
	for _, part := range strings.Split(s, "```") {
		if strings.Contains(part, "cmd/meshcoresim/") && strings.Contains(part, "internal/rf/") {
			return part
		}
	}
	return ""
}
