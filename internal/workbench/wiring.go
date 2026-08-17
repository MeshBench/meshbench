// The small types Run and the panels share: what a menu item is, and which
// role still needs a build chosen for it.
package workbench

import (
	"time"

	"github.com/MeshBench/meshbench/internal/gui/shell"
)

// menuAsks maps a verb to the one thing it needs before it can run.
//
// Keeping it beside the menu rather than inside the verb is deliberate: the
// verb is also driven by scripts, which supply the parameter directly and
// should not be made to answer a question.
var menuAsks = map[string]struct {
	title, hint, field string
	initial            func() string
}{
	"project.save": {
		title: "Save this network as",
		hint:  "a name, no extension",
		field: "name",
		initial: func() string {
			return "network-" + time.Now().Format("20060102-1504")
		},
	},
}

// roleNeed is one role with nodes that have nothing to run.
type roleNeed struct {
	role    string
	nodes   int
	choices []string
}

// menu is one menu bar heading and what is under it.
type menu struct {
	Name  string
	Items []shell.MenuItem
}
