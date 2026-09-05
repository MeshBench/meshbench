// The board's parts are drawn from one place, and a press on one is an edge.
//
// Two windows draw them - the node window's Hardware tab and the bring-up
// window - and the reason they are here rather than in either is that a second
// copy would be a second set of answers to "which pin is up". That took working
// out from two frames that disagree: the T-Deck's lines are named in the
// panel's portrait frame while the screen is drawn in landscape, so a pulse on
// the line called "up" moves the cursor down the picture.
package comp_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/ui/comp"
)

// An untouched board presses nothing, however often it is asked.
//
// Presses reports edges rather than levels, because the firmware counts changes
// of level - and a control that reported on its first frame would press every
// button on the board the moment the window opened, which on a board whose
// button is read active-low is a board that boots held down.
func TestAnUntouchedBoardPressesNothing(t *testing.T) {
	b, err := hw.BoardByName("LilyGo_TDeck")
	if err != nil {
		t.Fatal(err)
	}
	var c comp.BoardControls
	for i := 0; i < 3; i++ {
		if got := c.Presses(b.Hardware); len(got) != 0 {
			t.Fatalf("an untouched board reported %v on frame %d", got, i+1)
		}
	}
	// And a board that declares nothing pressable is not a crash.
	if got := c.Presses(nil); got != nil {
		t.Errorf("a board with no panel reported %v", got)
	}
}

// Every pin a board declares as pressable has a control, and the same pin
// always has the same one - widget identity is address, and a control rebuilt
// per frame loses the press half way through.
func TestOnePinKeepsOneControl(t *testing.T) {
	var c comp.BoardControls
	first := c.For(45)
	if second := c.For(45); second != first {
		t.Error("the same pin was given two controls, so a press begun on one " +
			"is finished on the other and neither sees a whole one")
	}
	if other := c.For(9); other == first {
		t.Error("two pins share a control, so pressing one presses the other")
	}
}

// Neither window keeps its own copy of the board part renderers.
//
// The Hardware tab had them and the bring-up window needed them; copying was
// the obvious move and would have put two answers to the same question in the
// tree. This is the check that keeps them in one place, in the same spirit as
// the shared widget helpers beside it.
func TestNoWindowKeepsItsOwnBoardPartRenderers(t *testing.T) {
	shared := map[string]string{
		"lamps":   "comp.BoardControls.Lamps",
		"lamp":    "comp.BoardControls.Lamps",
		"buttons": "comp.BoardControls.Buttons",
		"ball":    "comp.BoardControls.Ball",
	}
	root := filepath.Join("..", "workbench")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") || info.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if perr != nil {
			t.Errorf("%s does not parse, so it could not be checked: %v", path, perr)
			return nil
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil {
				continue
			}
			if want, is := shared[fn.Name.Name]; is {
				t.Errorf("%s declares its own %s; use %s.\n"+
					"Two windows draw a board's parts, and a second copy is a "+
					"second set of answers to which pin is up.",
					path, fn.Name.Name, want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
}
