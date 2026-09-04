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
// By path rather than by name, which is the correction that matters most. This
// keyed on the last segment alone, so one row anywhere in the map satisfied a
// directory of that name anywhere in the tree: internal/ui/workbench/packet was
// added with no row of its own and passed, silently answered by the row for
// internal/mesh/packet. A guard that passes for the wrong reason is worse than
// no guard, because it is trusted. The map's indentation already says where a
// row sits, so the path is rebuilt from it and compared whole.
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

// mapRow matches a row of the table: any indentation, a package name, a
// trailing slash, then whitespace and its description.
//
// The name may itself contain slashes, because a layer's own heading sits at
// column zero and spells its path out - "internal/rf/" - while the packages
// beneath it are indented and named by their last segment alone. Indentation
// is what carries depth, so any amount is accepted and the amount is what is
// read.
//
// A description that wraps onto another line cannot match: a continuation is
// prose, and prose has no bare "word/" before its first space. If one ever
// does, it builds a path that no directory answers, and the reverse check below
// fails loudly rather than quietly widening what counts as mapped.
var mapRow = regexp.MustCompile(`^( *)([a-z][a-z0-9-]*(?:/[a-z][a-z0-9-]*)*)/\s+\S`)

// layerEntry matches a layer's own heading, which sits at column zero as
// "internal/rf/" rather than being indented like the packages beneath it.
// Read by layers_test.go, which checks that the map is written in the order
// the layering is enforced in.
var layerEntry = regexp.MustCompile(`(?m)^internal/([a-z][a-z0-9]*)/`)

// skipDir is what is in the tree but is not part of the map: fixtures a package
// reads, and whatever a tool's interpreter leaves behind.
func skipDir(name string) bool {
	return strings.HasPrefix(name, "testdata") ||
		strings.HasPrefix(name, ".") ||
		strings.HasPrefix(name, "_")
}

// mappedPaths is every path the map carries a row for, rebuilt from its
// indentation.
//
// A row deeper than the one before it is a child of it; a row at the same
// indentation or shallower closes everything at least as deep. That is exactly
// what the table already means to a reader, said to the compiler.
func mappedPaths(block string) map[string]bool {
	type level struct {
		indent int
		path   string
	}
	var stack []level
	out := map[string]bool{}
	for _, line := range strings.Split(block, "\n") {
		m := mapRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		indent, name := len(m[1]), m[2]
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		path := name
		if len(stack) > 0 {
			path = stack[len(stack)-1].path + "/" + name
		}
		stack = append(stack, level{indent, path})
		out[path] = true
	}
	return out
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
			dirs[filepath.ToSlash(d)] = true
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
	mapped := mappedPaths(block)
	tracked := trackedDirs(t)
	found := map[string]bool{}

	// internal/ and tools/ are mapped to the bottom: a directory at any depth
	// is a thing somebody has to find, and the ones added last are the deepest.
	// pkg/ and cmd/ are mapped one level down, at the client and at the binary
	// rather than inside them.
	for _, root := range []struct {
		dir, prefix string
		deep        bool
	}{
		{".", "internal", true},
		{filepath.Join("..", "tools"), "tools", true},
		{filepath.Join("..", "pkg"), "pkg", false},
		{filepath.Join("..", "cmd"), "cmd", false},
	} {
		for _, rel := range dirsUnder(t, root.dir, root.deep) {
			path := root.prefix + "/" + rel
			if tracked != nil && !tracked[path] {
				continue
			}
			found[path] = true
			if !mapped[path] {
				t.Errorf("%s has no entry in CLAUDE.md's layout map.\n"+
					"A new package updates the map in the same commit - the map being "+
					"wrong is worse than the map being short.", path)
			}
		}
	}

	for path := range mapped {
		// The layer headings name themselves and have no directory of their
		// own to find - "internal/rf/" is a row and a directory both, and it is
		// walked as one of the roots above only for internal.
		if found[path] || isRoot(path) {
			continue
		}
		t.Errorf("CLAUDE.md's layout map names %q, which is not a directory in the tree.\n"+
			"A rename or a deletion left the map pointing at nothing.", path)
	}
}

// isRoot reports whether a mapped path is one of the tops the walk starts from
// rather than something below one.
func isRoot(path string) bool {
	switch path {
	case "internal", "tools", "pkg", "cmd":
		return true
	}
	// A layer heading - "internal/rf" - is a directory the walk does find, so
	// only the bare tops are excused here.
	return false
}

// dirsUnder walks root and returns every directory below it, relative to root
// and slash-separated. Shallow stops at one level, for the trees the map names
// at the client rather than inside it.
func dirsUnder(t *testing.T, root string, deep bool) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return fs.SkipAll
			}
			return err
		}
		if !d.IsDir() || path == root {
			return nil
		}
		if skipDir(d.Name()) {
			return fs.SkipDir
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		out = append(out, filepath.ToSlash(rel))
		if !deep {
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return out
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
