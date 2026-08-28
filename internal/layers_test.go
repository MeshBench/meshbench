// The layer rule, enforced.
//
// internal/ is nine layers deep, in this order, and a package may import its
// own layer and everything beneath it. Nothing may import upward. That is what
// makes "ui can reach the physics, the physics cannot reach a widget" a fact
// about the build rather than a claim in a document.
//
// This is here because a layout that is not enforced decays back. The tree
// already carried the shape - the layers verified against the real import
// graph with nothing bent to fit - and the only thing missing was something to
// notice when it stopped being true.
//
// It reads the source rather than shelling out to `go list`, for two reasons.
// A subprocess's file reads are invisible to Go's test cache, so a cached PASS
// would keep reporting a rule that had stopped being checked - which is worse
// than no rule. And parsing every file regardless of build tags means a
// violation cannot hide behind GOOS.
package internal_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// layers, lowest first. A package's layer is the first path element under
// internal/.
var layers = []string{"diag", "rf", "mesh", "firmware", "world", "sim", "study", "app", "ui"}

const modulePrefix = "github.com/MeshBench/meshbench/internal/"

func rank() map[string]int {
	r := make(map[string]int, len(layers))
	for i, l := range layers {
		r[l] = i
	}
	return r
}

// layerOf returns the layer a path sits in: "rf" for internal/rf/dsp, and
// false for anything not under one of the seven.
func layerOf(rel string, known map[string]int) (string, bool) {
	head, _, ok := strings.Cut(filepath.ToSlash(rel), "/")
	if !ok {
		return "", false
	}
	if _, isLayer := known[head]; !isLayer {
		return "", false
	}
	return head, true
}

func TestNoPackageImportsUpward(t *testing.T) {
	known := rank()
	fset := token.NewFileSet()
	var files, edges int

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		from, ok := layerOf(path, known)
		if !ok {
			return nil
		}
		files++
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			t.Errorf("parse %s: %v", path, perr)
			return nil
		}
		for _, spec := range f.Imports {
			p, uerr := strconv.Unquote(spec.Path.Value)
			if uerr != nil {
				continue
			}
			rest, isInternal := strings.CutPrefix(p, modulePrefix)
			if !isInternal {
				continue
			}
			to, ok := layerOf(rest, known)
			if !ok {
				continue
			}
			edges++
			if known[to] > known[from] {
				t.Errorf("%s imports %s\n\t%s is above %s, and the order is %s",
					path, p, to, from, strings.Join(layers, " < "))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if files == 0 {
		t.Fatal("no .go files found under internal/<layer>/ - has the layout moved?")
	}
	if !t.Failed() {
		t.Logf("%d files in the %d layers, %d internal imports, none pointing upward",
			files, len(layers), edges)
	}
}

// Every package under internal/ is in a layer. One that sits directly in
// internal/, or under a name that is not one of the seven, is invisible to the
// rule above - which is the quiet way a layering stops meaning anything.
func TestEveryInternalPackageIsInALayer(t *testing.T) {
	known := rank()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read internal/: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, ok := known[e.Name()]; !ok {
			t.Errorf("internal/%s is not one of %s", e.Name(), strings.Join(layers, ", "))
		}
	}
}
