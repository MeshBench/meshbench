// The layout map, enforced.
//
// CLAUDE.md carries a table of every directory under internal/, tools/ and
// pkg/ and what it holds, and states the rule in as many words: "A new package
// updates it in the same commit - the map being wrong is worse than the map
// being short." That rule was broken the week it was written, by a package that
// shipped without its line, and nobody noticed until somebody diffed the two by
// hand.
//
// A map is not read looking for the road that is missing. So this reads it
// instead, in both directions: a package with no entry fails, and an entry
// naming a package that has gone fails too - the second being the one a
// deletion or a rename leaves behind.
//
// To the full depth of the tree, because the shallow version of this check
// passed while internal/mesh/shim/repeater and internal/ui/workbench/licences
// were both absent from the map: they sit three deep, and a check that stops at
// two is a check that reports on the packages least likely to be new.
//
// Deliberately a test rather than a CI step. A CI step checks the tree that CI
// happens to run on; a test fails on the machine of whoever moved the package,
// while they still remember why.
package internal_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// mapEntry matches a line of the table: an indented package name, a trailing
// slash, then whitespace and its description. Indentation carries depth in the
// map and is not fixed, so any is accepted; the layer headings themselves start
// at column zero and so do not match.
var mapEntry = regexp.MustCompile(`(?m)^ +([a-z][a-z0-9-]*)/\s+\S`)

// layerEntry matches a layer's own heading, which sits at column zero as
// "internal/rf/" rather than being indented like the packages beneath it.
var layerEntry = regexp.MustCompile(`(?m)^internal/([a-z][a-z0-9]*)/`)

// skipDir is what is in the tree but is not part of the map: fixtures a package
// reads, and whatever a tool's interpreter leaves behind.
func skipDir(name string) bool {
	return strings.HasPrefix(name, "testdata") ||
		strings.HasPrefix(name, ".") ||
		strings.HasPrefix(name, "_")
}

// trackedDirs is every directory the repository actually keeps, asked of git
// rather than of the filesystem.
//
// The map describes the repository, so what is not in the repository has no
// row to be missing. Walking the filesystem instead meant an ignored build
// directory failed the build: tools/armfw/build is gitignored output, and
// anybody who had once built the ARM firmware got a red tree for a directory
// git does not track and the map should never mention.
//
// One invocation, and a nil answer if git cannot be reached, which leaves the
// walk as it was rather than passing a check it could not make.
func trackedDirs(t *testing.T) map[string]bool {
	t.Helper()
	out, err := exec.Command("git", "-C", "..", "ls-files", "-z").Output()
	if err != nil {
		t.Logf("git ls-files unavailable, checking every directory on disk: %v", err)
		return nil
	}
	dirs := map[string]bool{}
	for _, f := range strings.Split(string(out), "\x00") {
		for d := filepath.Dir(f); d != "." && d != string(filepath.Separator); d = filepath.Dir(d) {
			dirs[d] = true
		}
	}
	return dirs
}

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

	tracked := trackedDirs(t)
	found := map[string]bool{}
	// internal/ and tools/ are mapped to the bottom: a directory at any depth
	// is a thing somebody has to find, and the ones added last are the deepest.
	for _, root := range []string{".", filepath.Join("..", "tools")} {
		for _, dir := range dirsUnder(t, root) {
			if tracked != nil && !tracked[repoPath(root, dir)] {
				continue
			}
			found[filepath.Base(dir)] = true
			if !mapped[filepath.Base(dir)] {
				t.Errorf("%s has no entry in CLAUDE.md's layout map.\n"+
					"A new package updates the map in the same commit - the map being "+
					"wrong is worse than the map being short.", shown(root, dir))
			}
		}
	}

	// pkg/ is the public surface, and the map lists it at the client rather
	// than inside it: what a fork imports is client-go, not the files under it.
	if pkgs, err := os.ReadDir(filepath.Join("..", "pkg")); err == nil {
		for _, pkg := range pkgs {
			if !pkg.IsDir() || skipDir(pkg.Name()) {
				continue
			}
			found[pkg.Name()] = true
			if !mapped[pkg.Name()] {
				t.Errorf("pkg/%s has no entry in CLAUDE.md's layout map.\n"+
					"A new public package updates the map in the same commit.", pkg.Name())
			}
		}
	}

	for name := range mapped {
		if !found[name] {
			t.Errorf("CLAUDE.md's layout map names %q, which is not a directory in the tree.\n"+
				"A rename or a deletion left the map pointing at nothing.", name)
		}
	}
}

// dirsUnder walks root and returns every directory below it that the map is
// expected to carry a row for, deepest included.
func dirsUnder(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() || path == root {
			return nil
		}
		if skipDir(d.Name()) {
			return fs.SkipDir
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return out
}

// repoPath is a walked directory as git names it, from the repository root.
func repoPath(root, dir string) string {
	if root == "." {
		return filepath.Join("internal", strings.TrimPrefix(dir, "./"))
	}
	return filepath.Join("tools", strings.TrimPrefix(dir, filepath.Join("..", "tools")+string(filepath.Separator)))
}

// shown names a directory the way the map does, so the failure can be pasted
// straight into the row that is missing.
func shown(root, dir string) string {
	if root == "." {
		return "internal/" + filepath.ToSlash(dir)
	}
	return filepath.ToSlash(strings.TrimPrefix(dir, ".."+string(filepath.Separator)))
}

// layoutBlock returns the fenced code block that holds the map, found by the
// layer it must start with rather than by counting fences.
func layoutBlock(s string) string {
	for _, part := range strings.Split(s, "```") {
		if strings.Contains(part, "cmd/meshbench/") && strings.Contains(part, "internal/rf/") {
			return part
		}
	}
	return ""
}
