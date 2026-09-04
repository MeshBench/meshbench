// A verb that refuses without a window says so in the documentation.
//
// tools/verbdoc writes that mark, and CI regenerates the documentation and
// fails on a diff - so a wrong mark is caught only if verbdoc looked in the
// right place. It did not: ui_only listed the session directory while every
// scanner beside it walked the tree, so the first window verb to move into a
// domain package lost its mark and the reference said it works headless. It
// does not; it refuses.
//
// This is the cross-check, deliberately not sharing verbdoc's implementation.
// Two readers of the same source that must agree, in the same spirit as a GPU
// kernel and its CPU twin: a scanner that stops looking is invisible to
// itself, and only something that looks differently can see it.
package internal_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// handleVerb finds the verb name in a store registration.
var handleVerb = regexp.MustCompile(`(?:st|store)\.Handle(?:Internal)?\("([a-z0-9_.]+)"`)

func TestEveryVerbNeedingAWindowIsMarkedInTheDocs(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "docs", "scripting-verbs.md"))
	if err != nil {
		t.Fatalf("reading the verb table: %v", err)
	}
	table := string(doc)

	root := filepath.Join("app", "session")
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, verb := range guardedVerbs(string(src)) {
			// The row for this verb, with its mark if it has one. Matched on
			// the backticked name so a verb that is a prefix of another -
			// firmware.window against firmware.windows, were there one - does
			// not borrow its neighbour's mark.
			row := rowFor(table, verb)
			if row == "" {
				continue // not a public verb, so not in this table
			}
			if !strings.Contains(row, "🪟") {
				t.Errorf("%s guards on a window but its row in "+
					"docs/scripting-verbs.md carries no 🪟:\n  %s\n"+
					"Regenerate with tools/verbdoc/verbdoc.py. If the mark is "+
					"missing after that, verbdoc is not looking where this "+
					"verb now lives (%s).", verb, strings.TrimSpace(row), path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
}

// guardedVerbs is every verb in src whose handler asks for an interface first.
//
// Both spellings: a handler inside the session package calls the unexported
// needUI, and one in a domain package split out of it cannot, so it calls
// session.NeedUI. A scanner that knows only the first silently stops marking
// verbs the moment they move.
func guardedVerbs(src string) []string {
	var out []string
	for _, m := range handleVerb.FindAllStringSubmatchIndex(src, -1) {
		verb := src[m[2]:m[3]]
		body := handlerBody(src, m[1])
		if strings.Contains(body, "needUI()") || strings.Contains(body, "NeedUI()") {
			out = append(out, verb)
		}
	}
	return out
}

// handlerBody is the text from a registration to the end of its handler,
// found by counting braces from the first one.
func handlerBody(src string, from int) string {
	depth, start := 0, -1
	for i := from; i < len(src); i++ {
		switch src[i] {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start >= 0 {
				return src[start : i+1]
			}
			if depth < 0 {
				return ""
			}
		}
	}
	return ""
}

// rowFor is the table row naming this verb, or "" when the table has none.
func rowFor(table, verb string) string {
	for _, line := range strings.Split(table, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "| `"+verb+"`") {
			return line
		}
	}
	return ""
}
