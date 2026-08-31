package emulated

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/firmware"
)

// A node's name reaches a script Renode runs as monitor commands, and a name is
// not always typed by the operator: a scenario imported from a live feed
// carries whatever the network said. So a quote cannot end the argument it sits
// in, and a newline cannot start a command of its own.
func TestAHostileNodeNameCannotReachTheScript(t *testing.T) {
	hostile := []struct {
		name string
		what string
	}{
		{`a" ; include @/etc/passwd`, "a quote closing the argument"},
		{"a\nmach clear\nlogLevel 3", "a newline starting a command"},
		{`a\"b`, "an escaped quote"},
		{"a`b", "a backtick"},
		{"a$b", "a shell-looking expansion"},
	}
	for _, h := range hostile {
		t.Run(h.what, func(t *testing.T) {
			got := firmware.SafeNodeName(h.name)
			if strings.ContainsAny(got, "\"'`$\\\n\r ;") {
				t.Fatalf("%q survived as %q", h.name, got)
			}
			for _, r := range got {
				ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
					(r >= '0' && r <= '9') || r == '-' || r == '_'
				if !ok {
					t.Fatalf("unexpected rune %q in %q", r, got)
				}
			}
		})
	}
}

// Reduced, not discarded: a name still has to be recognisable in a script
// somebody is reading to work out which node misbehaved.
func TestAnOrdinaryNodeNameSurvives(t *testing.T) {
	for _, name := range []string{"Abernethy-Repeater", "sco_Goyle_Hill_r73", "node42"} {
		if got := firmware.SafeNodeName(name); got != name {
			t.Errorf("%q was changed to %q", name, got)
		}
	}
	// A space is not allowed through, but what is left still reads.
	if got := firmware.SafeNodeName("Bishop Hill"); got != "Bishop_Hill" {
		t.Errorf("wanted Bishop_Hill, got %q", got)
	}
}

// The working directory and the script take the same name through the same
// reduction, so the two cannot disagree about which node a file belongs to.
func TestTheScriptAndTheWorkingDirectoryAgree(t *testing.T) {
	name := `Bishop" Hill`
	dir := firmware.NodeWorkDir(name)
	if !strings.HasSuffix(dir, firmware.SafeNodeName(name)) {
		t.Fatalf("the work directory %q does not end in the safe name %q",
			dir, firmware.SafeNodeName(name))
	}
}
