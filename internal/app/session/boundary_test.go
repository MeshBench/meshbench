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
		"github.com/MeshBench/meshbench/internal/ui/comp",
		"github.com/MeshBench/meshbench/internal/ui/shell",
		"github.com/MeshBench/meshbench/internal/ui/theme",
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

// The store the session shares with the renderer holds no toolkit either.
//
// It used to live at internal/gui/state, which was the one entry in the allowed
// list above that surprised people: named for the interface, containing none of
// it. It is internal/app/state now, so the name and the dependency finally
// agree - but the assertion stays, because what mattered was never the path.
//
// Not a Skip when the file is missing. This test skipped silently the moment
// the package moved, which is the failure mode a boundary test cannot have.
func TestTheSnapshotPackageIsNotAToolkit(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "state", "state.go"))
	if err != nil {
		t.Fatalf("cannot read the store: %v", err)
	}
	if strings.Contains(string(src), "gioui.org") {
		t.Error("internal/app/state imports Gio, so the session depends on a toolkit through it")
	}
}
