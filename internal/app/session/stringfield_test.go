package session

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Does a bare string stay in the field it was passed for?
//
// It did not. stringField answers with the bare parameter whatever field it is
// asked for, which is right for the one field that parameter means and wrong
// for every other: ask it twice and both fields come back holding the same
// value. node.window is the case that surfaced it - the node arrives as a bare
// string, the optional tab was read with the same helper, and so every
// double-click asked for a tab named after the node and was refused by name.
// Nobody could open a node window at all.
//
// It was not one verb. Two dozen read a second field that way, and the ones
// that did not error just did something quietly wrong: firmware.import given a
// path took that path as its role, its board and its label too.
func TestABareStringFillsOnlyOneField(t *testing.T) {
	if got, _ := stringField("Jazzy", "tab"); got != "Jazzy" {
		t.Errorf("the primary field of a bare string is %q, want the string", got)
	}
	if got, ok := namedField("Jazzy", "tab"); ok || got != "" {
		t.Errorf("a second field of a bare string is %q/%v, want empty and absent", got, ok)
	}
	if got, ok := namedField(map[string]any{"tab": "Radio"}, "tab"); !ok || got != "Radio" {
		t.Errorf("a named field is %q/%v, want Radio", got, ok)
	}
}

// One primary per verb, checked rather than remembered.
//
// The rule only holds while every handler keeps to it, and the way it broke the
// first time was somebody reaching for the obvious helper twice in one verb. So
// this reads the handlers: a verb may call stringField once, and everything
// else it wants goes through namedField.
func TestEachVerbHasOneBareStringField(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Handle" {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			verb, _ := strconv.Unquote(lit.Value)
			var fields []string
			ast.Inspect(call.Args[1], func(n ast.Node) bool {
				inner, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				id, ok := inner.Fun.(*ast.Ident)
				if !ok || id.Name != "stringField" || len(inner.Args) != 2 {
					return true
				}
				if a, ok := inner.Args[1].(*ast.BasicLit); ok {
					s, _ := strconv.Unquote(a.Value)
					fields = append(fields, s)
				}
				return true
			})
			if len(fields) > 1 {
				t.Errorf("%s: %s reads %s with stringField; a bare string would fill all of them - "+
					"keep one as the primary and read the rest with namedField",
					name, verb, strings.Join(fields, ", "))
			}
			return true
		})
	}
}
