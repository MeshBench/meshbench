// What "Open a saved network" offers, and what choosing a row means.
package workbench

import (
	"path/filepath"
)

// builtInMark is what marks a shipped network in the list.
//
// A suffix rather than a section, because the chooser filters on the text of a
// row: typing "built" narrows to what shipped, and typing "fife" still finds
// it either way.
const builtInMark = " (built in)"

// openChoices turns project.list's reply into the rows to show and what each
// row opens.
//
// Two kinds of thing in one list, told apart by their label. The user's saved
// networks are files under their own configuration directory, so they open by
// path. A shipped one may exist only inside the binary, so it opens by name
// and lets the fixture search decide where that name lives - which is the same
// route -fixture takes and the only one that works on a machine with no
// fixtures on disk at all.
func openChoices(res any) ([]string, map[string]string) {
	m, _ := res.(map[string]any)
	dir, _ := m["dir"].(string)
	labels := []string{}
	opens := map[string]string{}
	for _, n := range listOfStrings(m["projects"]) {
		labels = append(labels, n)
		opens[n] = filepath.Join(dir, n+".json")
	}
	for _, n := range listOfStrings(m["fixtures"]) {
		label := n + builtInMark
		labels = append(labels, label)
		opens[label] = n
	}
	return labels, opens
}

// listOfStrings reads a verb's list of names whether it came back in process
// or through the socket, where a []string has been round-tripped into []any.
func listOfStrings(v any) []string {
	switch got := v.(type) {
	case []string:
		return got
	case []any:
		out := make([]string, 0, len(got))
		for _, e := range got {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
