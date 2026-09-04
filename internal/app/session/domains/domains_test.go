// Every domain package split out of session is listed here, or its verbs do
// not exist.
//
// A domain hangs its verbs off the store from its own init, and an init only
// runs if something imports the package. This aggregator is that something, so
// a package added under session/ and not added here compiles, tests green, and
// registers nothing: the verbs are simply absent at runtime, and the first
// report is a script or a panel getting "unknown verb" for something that is
// plainly in the tree.
//
// That is not hypothetical - it is the failure mode the experiment split would
// have had, since every one of its eleven verbs moved behind an init in one
// commit. This walk is cheap and says which package was forgotten.
package domains

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEveryDomainPackageIsBlankImportedHere(t *testing.T) {
	const aggregator = "domains.go"

	src, err := os.ReadFile(aggregator)
	if err != nil {
		t.Fatalf("reading %s: %v", aggregator, err)
	}
	listed := string(src)

	root := filepath.Join("..")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("reading %s: %v", root, err)
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "domains" || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if !registersADomain(t, dir) {
			continue // a helper package, not a verb domain
		}
		want := `session/` + e.Name() + `"`
		if !strings.Contains(listed, want) {
			t.Errorf("%s calls session.RegisterDomain but %s does not import it.\n"+
				"Its verbs will not be registered: an init only runs if something "+
				"imports the package, and this aggregator is that something.",
				dir, aggregator)
		}
	}
}

// registersADomain reports whether the package calls session.RegisterDomain,
// which is what makes it a domain rather than a package session happens to
// contain.
func registersADomain(t *testing.T, dir string) bool {
	t.Helper()
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Errorf("reading %s: %v", dir, err)
		return false
	}
	for _, f := range files {
		name := f.Name()
		if f.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("reading %s: %v", path, err)
			continue
		}
		// Parsed rather than grepped, so the phrase in a comment explaining the
		// pattern is not mistaken for a package that uses it - which is exactly
		// what a doc comment on a domain package tends to contain.
		if _, err := parser.ParseFile(token.NewFileSet(), path, src, 0); err != nil {
			t.Errorf("%s does not parse, so it could not be checked: %v", path, err)
			continue
		}
		if strings.Contains(stripComments(t, path, src), "session.RegisterDomain(") {
			return true
		}
	}
	return false
}

// stripComments returns the file's source with its comments removed, so a
// mention of the call in prose does not count as the call.
func stripComments(t *testing.T, path string, src []byte) string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return string(src)
	}
	out := append([]byte(nil), src...)
	base := fset.File(file.Pos()).Base()
	for _, group := range file.Comments {
		for i := int(group.Pos()) - base; i < int(group.End())-base && i < len(out); i++ {
			out[i] = ' '
		}
	}
	return string(out)
}
