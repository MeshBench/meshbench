// The shared widget helpers live here, and nowhere else.
//
// Fixed, CopyText and BorderedAction were each declared inside whichever panel
// first wanted them - two in the packet inspector, one in firmwarepanel.go -
// and then used from across the interface. That is invisible until a panel
// moves: splitting the inspector out took two helpers the companion panels
// depend on with it, and the package stopped compiling for a reason that had
// nothing to do with the split.
//
// A helper half the interface calls belongs where the widgets live. This is the
// check that keeps it there, because the pull to declare a small helper beside
// its first caller is constant and the cost only appears later.
package comp_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shared are the helpers that now live in comp. A package under internal/ui
// declaring its own function by one of these names is either shadowing the
// shared one or reintroducing it.
var shared = map[string]string{
	"fixed":            "comp.Fixed",
	"copyText":         "comp.CopyText",
	"borderedAction":   "comp.BorderedAction",
	"fieldText":        "comp.FieldText",
	"selectedNodeName": "comp.SelectedNodeName",
	"splitFields":      "comp.SplitFields",
}

func TestNoPanelKeepsItsOwnCopyOfASharedHelper(t *testing.T) {
	root := filepath.Join("..", "..", "ui")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "testdata" {
				return filepath.SkipDir
			}
			// comp is where they are meant to be.
			if path == filepath.Join(root, "comp") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			// Said rather than swallowed. A file that does not parse is
			// another test's failure to report, but skipping it silently here
			// would let a helper hide inside one - the walk would simply not
			// look, and this test would pass for the wrong reason.
			t.Errorf("%s does not parse, so it could not be checked: %v", path, parseErr)
			return nil
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil {
				continue // a method of that name is a different thing
			}
			if want, isShared := shared[fn.Name.Name]; isShared {
				t.Errorf("%s declares its own %s; use %s.\n"+
					"A helper the rest of the interface also calls does not belong "+
					"in the panel that wanted it first - the next panel to move "+
					"takes it with them.", path, fn.Name.Name, want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
}
