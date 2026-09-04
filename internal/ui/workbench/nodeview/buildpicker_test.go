// Every build in a long library is reachable by filtering.
//
// Moved here with the node view: it drives ViewPanel's own build picker
// through its internals, which is a test of this package rather than of the
// workbench's control audit, where it used to sit.
package nodeview

import (
	"fmt"
	"testing"

	"gioui.org/f32"
	"github.com/MeshBench/meshbench/internal/ui/uitest"
)

// The build you want is not always one of the eleven that fit.
// longLibrary is more builds than the overlay can show at once, so the last of
// them is genuinely below the fold and has to be filtered for.
func longLibrary() []BuildChoice {
	var out []BuildChoice
	for i := 40; i > 20; i-- {
		v := fmt.Sprintf("v1.%d.0", i)
		out = append(out, BuildChoice{Label: v, Version: v})
	}
	return out
}

func TestAnyBuildIsReachableByFiltering(t *testing.T) {
	nv := &ViewPanel{}
	// A library long enough to overflow the overlay, supplied rather than
	// found. This test used to read the machine's own and skip itself when it
	// held fewer than twelve builds - which on a clean runner is always, so the
	// one test standing behind the audit's "reached by filtering" exemption
	// did not run in CI at all. Between them the two tests excused the tail of
	// the list to each other and neither walked it.
	nv.pick.library = longLibrary
	nv.pick.open("Abernethy Repeater")
	got := ""
	nv.OnFirmware = func(node string, b BuildChoice) { got = b.Version }

	h := uitest.New(nv.Draw, uitest.Snapshot())
	h.Frame()
	h.Frame()
	if len(nv.pick.builds) < 12 {
		t.Fatalf("the supplied library holds %d builds; this test needs enough"+
			" to put one below the fold", len(nv.pick.builds))
	}
	want := nv.pick.builds[len(nv.pick.builds)-1]

	// Type enough of its name to leave it alone in the list.
	nv.pick.filter.Editor.SetText(want.Label)
	h.Frame()

	// Upward, because cancel sits at the top of the card and closing the list
	// on the way to the thing inside it proves nothing.
	for y := float32(h.Size.Y) - 2; y > 2 && got == ""; y -= 4 {
		for x := float32(2); x < float32(h.Size.X) && got == ""; x += 8 {
			h.Click(f32.Pt(x, y))
		}
	}
	if got != want.Version {
		t.Fatalf("filtered the build list to %q and clicking it reached %q; "+
			"a build that does not fit on screen has to be reachable by "+
			"narrowing the list", want, got)
	}
}
