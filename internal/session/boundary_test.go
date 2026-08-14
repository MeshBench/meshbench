package session

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The line this package exists to hold.
//
// Nothing here may import a user interface toolkit. The point of separating the
// session from the front end is that choosing a front end stays a choice, and
// the way that quietly stops being true is one import at a time - a panel type
// borrowed for convenience, a layout constant, a colour. So it is a test rather
// than a comment.
func TestTheSessionKnowsNothingAboutAnyToolkit(t *testing.T) {
	banned := []string{
		"gioui.org",
		"github.com/AllenDang/cimgui-go",
		"github.com/MeshBench/meshbench/internal/gui/comp",
		"github.com/MeshBench/meshbench/internal/gui/shell",
		"github.com/MeshBench/meshbench/internal/gui/theme",
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 5 {
		t.Fatalf("only %d files found; the test is looking in the wrong place", len(files))
	}
	fset := token.NewFileSet()
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.ParseFile(fset, f, src, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, imp := range parsed.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			for _, bad := range banned {
				if path == bad || strings.HasPrefix(path, bad+"/") {
					t.Errorf("%s imports %s\n"+
						"the session must not know about a user interface; "+
						"if the change needs one, it belongs on the other side of the line",
						f, path)
				}
			}
		}
	}
}

// internal/gui/state is allowed, and is the one that would surprise somebody.
//
// It is named for the interface and contains none of it: a snapshot, a store
// and the verbs' world. The name is wrong rather than the dependency, and
// asserting that here means somebody removing it from the allowed list has to
// think about which of those two they are fixing.
func TestTheSnapshotPackageIsNotAToolkit(t *testing.T) {
	src, err := os.ReadFile("../gui/state/state.go")
	if err != nil {
		t.Skip("state package not where expected")
	}
	if strings.Contains(string(src), "gioui.org") {
		t.Error("internal/gui/state imports Gio, so the session depends on a toolkit through it")
	}
}
