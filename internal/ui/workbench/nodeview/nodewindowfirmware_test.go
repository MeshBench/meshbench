package nodeview

import (
	"strings"
	"testing"

	"gioui.org/f32"
	"github.com/MeshBench/meshbench/internal/ui/uitest"
)

// Changing one node's build, from the window that is about that node.
//
// node.set_firmware has always applied correctly and was only ever reachable
// from the Nodes running table or over the control socket, so pinning a local
// build to one node meant leaving the window naming it. These press the way
// somebody would: find the button, then pick from what it opens.

func nodeWindowWithBuilds(t *testing.T) (*WindowPanel, *uitest.Harness) {
	t.Helper()
	p := &WindowPanel{Node: "Abernethy Repeater", Kind: "repeater"}
	h := uitest.New(p.Draw, uitest.Snapshot())
	p.Tab = TabSettings
	h.Frame()
	h.Frame() // labels are built on the first
	// The picker reads the library when it is drawn, and it is not drawn
	// until something opens it. Asked for here so the test can say what it
	// expects to be able to choose.
	p.pick.load()
	if len(p.pick.builds) == 0 {
		t.Skip("no native builds installed on this machine")
	}
	return p, h
}

// The button exists, is reachable by a pointer, and opens the list.
func TestTheSettingsTabOpensTheBuildList(t *testing.T) {
	p, h := nodeWindowWithBuilds(t)
	if p.pick.showing() {
		t.Fatal("the build list was already open before anything was pressed")
	}
	// Down the left of the Settings pane, where the section's controls sit.
	for y := float32(2); y < float32(h.Size.Y) && !p.pick.showing(); y += 4 {
		h.Click(f32.Pt(80, y))
	}
	if !p.pick.showing() {
		t.Fatal("nothing on the Settings tab opens a build list")
	}
	if p.pick.node != p.Node {
		t.Fatalf("the list opened for %q, want %q", p.pick.node, p.Node)
	}
}

// And choosing from it reaches the verb that stops, provisions and restarts.
func TestTheNodeWindowChangesOneNodesFirmware(t *testing.T) {
	p, h := nodeWindowWithBuilds(t)
	verb, params := "", map[string]any(nil)
	p.OnDo = func(v string, ps any) {
		verb, _ = v, ps
		params, _ = ps.(map[string]any)
	}
	want := p.pick.builds[0]
	p.pick.open(p.Node)
	// One build only, so the click cannot land on a neighbour.
	p.pick.filter.Editor.SetText(want.Label)
	h.Frame()

	// Upward: cancel sits at the top of the card, and closing the list on the
	// way to the thing inside it proves nothing.
	for y := float32(h.Size.Y) - 2; y > 2 && verb == ""; y -= 4 {
		for x := float32(2); x < float32(h.Size.X) && verb == ""; x += 8 {
			h.Click(f32.Pt(x, y))
		}
	}
	if verb != "node.set_firmware" {
		t.Fatalf("choosing a build reached %q, want node.set_firmware", verb)
	}
	if params["node"] != p.Node {
		t.Fatalf("set_firmware asked about %v, want %q", params["node"], p.Node)
	}
	// One of the builds the filter left showing, rather than the exact one:
	// build names nest - companion-v1.16.0 is a prefix of
	// companion-v1.16.0-faultyirq - so a filter that leaves two is not a
	// fault in the control.
	got, _ := params["version"].(string)
	if !strings.Contains(got, want.Label) {
		t.Fatalf("filtered the list to %q and the click chose %q", want.Label, got)
	}
}

// set_firmware, not set_firmware_only: a build recorded and not applied is the
// control somebody presses twice and then distrusts.
func TestChangingFirmwareAppliesItRatherThanRecordingIt(t *testing.T) {
	p := &WindowPanel{Node: "Abernethy Repeater"}
	verb := ""
	p.OnDo = func(v string, _ any) { verb = v }
	h := uitest.New(p.Draw, uitest.Snapshot())
	h.Frame()
	p.pick.OnPick(p.Node, BuildChoice{Label: "repeater-v1.17.0", Version: "repeater-v1.17.0"})
	if verb != "node.set_firmware" {
		t.Fatalf("picking a build reached %q, want node.set_firmware", verb)
	}
}
