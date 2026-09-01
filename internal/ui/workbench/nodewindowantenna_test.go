package workbench

import (
	"testing"

	"gioui.org/f32"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// Choosing and aiming an antenna, from the window that names the node.
//
// The audit proves every control here reaches something; these say which verb
// it reaches and with what, because "reaches a verb" and "reaches the right
// verb with the node's own name on it" are different claims and only the second
// one is the feature.

func antennaWindow(t *testing.T) (*nodeWindowPanel, *panelHarness, *[]antennaCall) {
	t.Helper()
	snap := auditSnapshot()
	for i := range snap.Nodes {
		if snap.Nodes[i].Name == "Abernethy Repeater" {
			snap.Nodes[i].Antenna = state.Antenna{
				Type: "collinear", GainDBiPeak: 6.15,
				Polarisation: "vertical", FeedlineDB: 0.8,
			}
		}
	}
	p := &nodeWindowPanel{node: "Abernethy Repeater", Kind: "repeater"}
	var calls []antennaCall
	p.OnDo = func(v string, ps any) {
		m, _ := ps.(map[string]any)
		calls = append(calls, antennaCall{verb: v, params: m})
	}
	p.tab = tabAntenna
	h := newPanelHarness(p.Draw, snap)
	h.frame()
	h.frame() // labels and boxes are built on the first
	return p, h, &calls
}

// antennaCall is one verb this tab fired, with the parameters it carried. Its
// own type because the firmware window's call keeps its parameters as an any,
// and every assertion here is about a named key of the map.
type antennaCall struct {
	verb   string
	params map[string]any
}

// The tab exists on an ordinary repeater's window, and opens on the antenna.
func TestANodeWindowHasAnAntennaTab(t *testing.T) {
	p := &nodeWindowPanel{node: "Abernethy Repeater", Kind: "repeater"}
	found := false
	for _, tb := range p.visibleTabs() {
		if tb == tabAntenna {
			found = true
		}
	}
	if !found {
		t.Fatalf("a repeater's window offers %v and no antenna", p.visibleTabs())
	}
	if got := tabAntenna.String(); got != "Antenna" {
		t.Errorf("the tab is called %q", got)
	}
}

// Pressing a sort applies it to this node, immediately: the map beside the
// window is the feedback, and a form that waits for a submit gives none.
func TestChoosingASortAppliesItToThisNode(t *testing.T) {
	p, h, calls := antennaWindow(t)
	p.ant.sorts[3].Click.Click() // the beam
	h.frame()
	h.frame()
	if len(*calls) == 0 {
		t.Fatal("choosing a sort reached nothing")
	}
	got := (*calls)[0]
	if got.verb != "nodes.antenna" {
		t.Fatalf("choosing a sort reached %q", got.verb)
	}
	if got.params["node"] != "Abernethy Repeater" {
		t.Errorf("it named %v, not the node the window is about", got.params["node"])
	}
	if got.params["pattern"] != "yagi" {
		t.Errorf("it asked for %v, not a yagi", got.params["pattern"])
	}
}

// And a polarisation, which is the field that had no consumer at all before.
func TestChoosingAPolarisationAppliesIt(t *testing.T) {
	p, h, calls := antennaWindow(t)
	p.ant.pols[1].Click.Click() // horizontal
	h.frame()
	h.frame()
	if len(*calls) == 0 {
		t.Fatal("choosing a polarisation reached nothing")
	}
	if got := (*calls)[0]; got.params["polarisation"] != "horizontal" {
		t.Errorf("it asked for %v", got.params["polarisation"])
	}
}

// The two apply buttons are different buttons: one names this node and one
// deliberately does not, which is what makes the second the fleet default.
func TestApplyToEveryNodeLeavesTheNodeOff(t *testing.T) {
	p, h, calls := antennaWindow(t)
	p.ant.bearing.Editor.SetText("217")
	h.frame()

	p.ant.apply.Click.Click()
	h.frame()
	h.frame()
	p.ant.applyAll.Click.Click()
	h.frame()
	h.frame()

	if len(*calls) != 2 {
		t.Fatalf("two presses reached %d verbs", len(*calls))
	}
	one, all := (*calls)[0], (*calls)[1]
	if one.params["node"] != "Abernethy Repeater" {
		t.Errorf("apply named %v", one.params["node"])
	}
	if _, named := all.params["node"]; named {
		t.Error("apply to every node named a node, so it would change only that one")
	}
	for _, c := range []antennaCall{one, all} {
		if c.params["bearing_deg"] != 217.0 {
			t.Errorf("%v carried bearing %v, not what is in the box",
				c.verb, c.params["bearing_deg"])
		}
	}
}

// An empty box means "leave it alone", not "set it to zero". A collinear has no
// beamwidth, so the box is empty, and sending 0 would be a claim about an
// antenna with an infinitely narrow lobe.
func TestAnEmptyBoxIsNotSentAsZero(t *testing.T) {
	p, h, calls := antennaWindow(t)
	p.ant.apply.Click.Click()
	h.frame()
	h.frame()
	if len(*calls) == 0 {
		t.Fatal("apply reached nothing")
	}
	got := (*calls)[0]
	if _, sent := got.params["beamwidth_deg"]; sent {
		t.Errorf("an empty beamwidth was sent as %v", got.params["beamwidth_deg"])
	}
	// What the node does have still goes.
	if got.params["gain_dbi_peak"] != 6.15 {
		t.Errorf("the gain went as %v, not the node's own", got.params["gain_dbi_peak"])
	}
}

// Aiming carries the name in the box, which is the whole of what that control
// is for.
func TestAimingCarriesTheNameInTheBox(t *testing.T) {
	p, h, calls := antennaWindow(t)
	p.ant.aimAt.Editor.SetText("Bishop Hill")
	h.frame()
	p.ant.aim.Click.Click()
	h.frame()
	h.frame()
	if len(*calls) == 0 {
		t.Fatal("point it there reached nothing")
	}
	got := (*calls)[0]
	if got.verb != "node.aim" {
		t.Fatalf("aiming reached %q", got.verb)
	}
	if got.params["node"] != "Abernethy Repeater" || got.params["at"] != "Bishop Hill" {
		t.Errorf("it aimed %v at %v", got.params["node"], got.params["at"])
	}
}

// A bearing computed from two positions is a full float, and the box is where
// somebody edits it. Seventeen significant figures is not a number anybody can
// change, and it pushes the units off the end of the field.
func TestTheBearingBoxIsANumberSomebodyCanEdit(t *testing.T) {
	if got := number(280.3928823597621); got != "280.39" {
		t.Errorf("a computed bearing shows as %q", got)
	}
	if got := number(180); got != "180" {
		t.Errorf("a round bearing shows as %q, with trailing noise", got)
	}
	if got := number(0.8); got != "0.8" {
		t.Errorf("a feedline loss shows as %q", got)
	}
}

// The tab is reachable by a pointer, not only by setting the field: a tab
// nobody can click is a tab nobody has.
func TestTheAntennaTabCanBeClicked(t *testing.T) {
	p, h, _ := antennaWindow(t)
	p.tab = tabConsole
	h.frame()
	// Across the head of the window, where the strip is. Both axes rather than
	// one guessed row: the strip sits under a title, a status line and a
	// subtitle whose heights are the theme's to decide.
	for y := float32(2); y < 220 && p.tab != tabAntenna; y += 3 {
		for x := float32(2); x < 700 && p.tab != tabAntenna; x += 4 {
			h.click(f32.Pt(x, y))
		}
	}
	if p.tab != tabAntenna {
		t.Fatal("no pointer along the tab strip lands on Antenna")
	}
}
