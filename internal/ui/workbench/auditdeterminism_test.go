package workbench

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Does the audit walk the same controls whatever is installed on the machine?
//
// It did not. The build picker read the machine's own firmware library, so the
// number of controls the audit walked was a property of the runner: a warm
// cache had more buttons than a cold one, the skip list excused everything past
// a hardcoded tenth, and whether the tenth itself fell below the fold depended
// on the face the interface happened to be set in. The same commit passed and
// failed, and the failure named a file in somebody's cache:
//
//	no pointer lands on it: pick.btns[10] (imported-20260823-035641)
//
// This runs the walk against an empty library and then against twenty builds
// on disk, and requires the same controls both times. It is the only reason
// the audit can be a gate: a test that reports a different answer for the same
// tree is not gating anything.
func TestTheAuditDoesNotReadTheMachine(t *testing.T) {
	// Drawn, not just constructed: the build list fills itself during layout,
	// so a walk that never lays the panel out finds no buttons on any machine
	// and would pass whatever leaked.
	snap := auditSnapshot()
	walk := func() []string {
		var out []string
		for _, tg := range auditTargets(&recorder{}) {
			use := snap
			if tg.snap != nil {
				use = tg.snap
			}
			h := newPanelHarness(tg.draw, use)
			h.frame()
			h.frame()
			var found []control
			controlsOf(reflect.ValueOf(tg.ctrl), "", &found)
			for _, c := range found {
				out = append(out, tg.name+"."+c.name)
			}
		}
		return out
	}

	// A machine with nothing installed.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	empty := walk()
	if len(empty) == 0 {
		t.Fatal("no controls found at all; the walk is broken, not the seam")
	}

	// The same machine with a library on it, in the layout ListInstalled
	// reads: one directory per version, holding a meshcore- binary.
	cache := t.TempDir()
	for i := 1; i <= 20; i++ {
		dir := filepath.Join(cache, "meshbench", "firmware", "native",
			fmt.Sprintf("v1.%d.0", i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "meshcore-repeater"),
			[]byte("not a real firmware"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_CACHE_HOME", cache)

	// The fabrication has to be real enough to have leaked, or this proves
	// nothing: check the reader does see it before asking whether the audit
	// does not.
	if got := len(installedBuilds()); got != 20 {
		t.Fatalf("the fabricated library reads as %d builds, want 20 -"+
			" this test cannot detect a leak it cannot cause", got)
	}

	if full := walk(); !reflect.DeepEqual(empty, full) {
		t.Errorf("the audit walks %d controls with an empty cache and %d with"+
			" twenty builds installed, so its verdict depends on the machine",
			len(empty), len(full))
	}
}
