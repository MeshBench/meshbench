package session

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// handlerBodies is every verb registered anywhere under this package, with the
// source of its handler.
//
// Read out of the files rather than off the store, because the store holds
// closures and the question here is what the closure was written to read. It
// walks the subdirectories as well: the split-out domains register verbs too,
// and a rule enforced over one directory is a rule half the surface escapes.
func handlerBodies(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range registerCall.FindAllStringSubmatchIndex(string(src), -1) {
			verb := string(src[m[2]:m[3]])
			out[verb] = bodyAt(string(src), m[1])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	helpers := paramHelpers(t)
	for verb, body := range out {
		out[verb] = body + helpersReached(body, helpers)
	}
	return out
}

// paramHelpers is every function in the package that is handed a verb's whole
// parameter, by name.
//
// A handler that passes p on has not stopped reading it, and a rule that looked
// only at the literal would let a verb escape by moving one line into a
// function - which is what coverage.map has already done with its own borders.
func paramHelpers(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range funcDecl.FindAllStringSubmatchIndex(string(src), -1) {
			out[string(src[m[2]:m[3]])] = bodyAt(string(src), m[1])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

var funcDecl = regexp.MustCompile(`(?m)^func (?:\([^)]*\) )?([A-Za-z0-9_]+)\(`)

// helpersReached is the source of every helper the handler hands p to, and of
// every helper those hand it on to in turn.
//
// Followed to a fixed point rather than one call deep: coverage.map passes its
// whole parameter through two functions before anything reads a field out of
// it, and a check that stopped at the first would call that verb a fault.
func helpersReached(body string, helpers map[string]string) string {
	var out strings.Builder
	seen := map[string]bool{}
	for frontier := []string{body}; len(frontier) > 0; {
		next := frontier[len(frontier)-1]
		frontier = frontier[:len(frontier)-1]
		for _, m := range passesParam.FindAllStringSubmatch(next, -1) {
			src, ok := helpers[m[1]]
			if !ok || seen[m[1]] {
				continue
			}
			seen[m[1]] = true
			out.WriteString(src)
			frontier = append(frontier, src)
		}
	}
	return out.String()
}

var passesParam = regexp.MustCompile(`(?:[A-Za-z0-9_]+\.)?([A-Za-z0-9_]+)\(` +
	`[^()]*\bp\b[^()]*\)`)

var registerCall = regexp.MustCompile(`st\.Handle(?:Internal)?\(\s*"([a-z0-9_.]+)"\s*,`)

// bodyAt is the handler literal that starts at or after from, by brace
// matching. Strings and raw strings are skipped so a brace inside a refusal
// message does not end the body early.
func bodyAt(src string, from int) string {
	i := strings.Index(src[from:], "{")
	if i < 0 {
		return ""
	}
	i += from
	depth := 0
	for j := i; j < len(src); j++ {
		switch src[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[i : j+1]
			}
		case '"':
			for j++; j < len(src) && src[j] != '"'; j++ {
				if src[j] == '\\' {
					j++
				}
			}
		case '`':
			if k := strings.Index(src[j+1:], "`"); k >= 0 {
				j += k + 1
			}
		}
	}
	return src[i:]
}

// readsByName reports whether a handler reads one named field with one of the
// readers that actually looks the name up.
func readsByName(body, name string, readers []string) bool {
	// Either shape a reader is called in: the parameter first, or the verb's
	// own name first for the readers whose refusals say which verb they came
	// from. The field name is the argument after it in both.
	// The field name is always the argument after the first, whether that first
	// one is the parameter itself or the verb's own name - which several of the
	// readers take so their refusals can say where they came from, and which
	// arrives as a constant as often as a literal.
	pat := regexp.MustCompile(`\b(?:` + strings.Join(readers, "|") +
		`)\(\s*[^,()]*,\s*"` + regexp.QuoteMeta(name) + `"`)
	// The third shape is a handler that unwrapped the object itself and
	// subscripts it, whatever it called the map it got.
	unwrapped := regexp.MustCompile(`[A-Za-z0-9_]+\["` + regexp.QuoteMeta(name) + `"\]`)
	return pat.MatchString(body) || unwrapped.MatchString(body)
}

// The readers a PRIMARY parameter may be read with: the ones that accept a
// bare value as well as a named one.
//
// The named-only readers are deliberately absent. A primary parameter is the
// one a bare value may mean, so reading it with namedNum or namedField would
// look up the right name and still refuse the bare form the description
// promises - the same disagreement the other way round.
var stringReaders = []string{"stringField", "StringField",
	"primaryString", "PrimaryString"}
var numberReaders = []string{"numField", "NumField", "numAsked", "NumAsked",
	"numInRange", "NumInRange"}

// Does a verb read the parameter its own description promises?
//
// Four did not. node.energy, coverage.compute, node.stop and node.start all
// documented a required primary `node` and then read soleString, which answers
// with the only value of any single-key object and never looks the name up: pass
// the documented key alongside anything else and the verb saw no node at all and
// refused, naming the empty string. node.window went further and overwrote the
// name it had with m["node"] unconditionally, so {"tab": "Radio"} emptied it.
//
// Checked here rather than remembered, because the description and the handler
// are written in two files and nothing else compares them. A primary parameter
// is the one a bare value may mean; it is not permission to stop reading the
// name.
func TestEveryDocumentedPrimaryIsReadByName(t *testing.T) {
	specs, err := state.LoadSpecs(filepath.Join("..", "..", "..", "internal"))
	if err != nil {
		t.Fatal(err)
	}
	bodies := handlerBodies(t)
	var bad []string
	for verb, spec := range specs {
		body, ok := bodies[verb]
		if !ok {
			continue
		}
		for _, prm := range spec.Params {
			if !prm.Primary {
				continue
			}
			var readers []string
			switch prm.Type {
			case state.ParamString:
				readers = stringReaders
			case state.ParamNumber:
				readers = numberReaders
			default:
				continue
			}
			if !readsByName(body, prm.Name, readers) {
				bad = append(bad, fmt.Sprintf("%s documents %q and never reads it",
					verb, prm.Name))
			}
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Errorf("%d verbs publish a parameter they do not look up; read it by "+
			"name and fall back to the bare value, as primaryString does:\n  %s",
			len(bad), strings.Join(bad, "\n  "))
	}
}

// Does a bare number stay in the field it was passed for?
//
// The rule stringfield_test.go keeps for strings, kept for numbers by the same
// reading of the same handlers. numField answers with the bare parameter
// whatever field it is asked for, so nodes.place reading lat and lon through it
// turned a bare 5 into a node at 5N 5E: two coordinates invented out of one, and
// both reported back as asked for.
func TestEachVerbHasOneBareNumberField(t *testing.T) {
	bare := regexp.MustCompile(
		`\b(?:NumField|numField|NumAsked|numAsked|NumInRange|numInRange)` +
			`\(\s*[^,()]*,\s*"([a-z0-9_]+)"`)
	var bad []string
	for verb, body := range handlerBodies(t) {
		seen := map[string]bool{}
		for _, m := range bare.FindAllStringSubmatch(body, -1) {
			seen[m[1]] = true
		}
		if len(seen) > 1 {
			names := make([]string, 0, len(seen))
			for n := range seen {
				names = append(names, n)
			}
			sort.Strings(names)
			bad = append(bad, verb+" reads "+strings.Join(names, ", "))
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Errorf("%d verbs let one bare number fill several fields; keep one as "+
			"the primary and read the rest with namedNum:\n  %s",
			len(bad), strings.Join(bad, "\n  "))
	}
}
