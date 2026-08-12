package ui

import (
	"slices"
	"testing"

	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// A permissive fixture has to survive being saved and loaded, and the thing
// that makes it permissive is a line typed at a node's console. Console lines
// are not saved, so the first attempt shipped a "permissive" fixture that was
// byte-identical to the strict one - the difference existed only in a running
// process, and both variants forwarded the same traffic.
//
// This is the guard: the state lives on the node, so the provisioning that runs
// at every firmware start rebuilds it.
func TestPermissiveNodeIsToldToForwardAnyRegion(t *testing.T) {
	a := &App{Nodes: []scenario.Node{{
		Name: "R", Kind: scenario.SimpleRepeater, AllowAnyFlood: true,
	}}}
	// Already initialised, so provisioning is not read from the operator's own
	// configuration file: what this asserts is a property of the node.
	a.cfg.init = true
	got := a.regionCommands(0)
	if !slices.Contains(got, "region allowf *") {
		t.Fatalf("a permissive node was told %v, with no wildcard among it", got)
	}
	// Without the save it is gone at the next boot, which is the same silence
	// in slower motion.
	if !slices.Contains(got, "region save") {
		t.Errorf("the wildcard was allowed but never saved: %v", got)
	}
}

// A node that holds real regions and is not permissive must not be given the
// wildcard, or every fixture would quietly become the generous one.
func TestStrictNodeIsNotToldToForwardAnyRegion(t *testing.T) {
	a := &App{Nodes: []scenario.Node{{
		Name: "R", Kind: scenario.SimpleRepeater, Regions: []string{"#sco"},
	}}}
	a.cfg.init = true
	if got := a.regionCommands(0); slices.Contains(got, "region allowf *") {
		t.Errorf("a strict node was told to forward any region: %v", got)
	}
}

// An observer transmits nothing, so it has no forwarding to permit. Counting it
// would make "3 of 4 nodes" mean something the operator cannot act on.
func TestOnlyTransmittingNodesCount(t *testing.T) {
	a := &App{Nodes: []scenario.Node{
		{Name: "R", Kind: scenario.SimpleRepeater},
		{Name: "C", Kind: scenario.Companion},
		{Name: "O", Kind: scenario.SDRObserver},
	}}
	if changed := a.setAnyFlood(true); changed != 2 {
		t.Errorf("setAnyFlood changed %d nodes, want 2", changed)
	}
	on, total := a.anyFloodState()
	if on != 2 || total != 2 {
		t.Errorf("anyFloodState() = %d of %d, want 2 of 2", on, total)
	}
	if a.Nodes[2].AllowAnyFlood {
		t.Error("an SDR observer was made permissive; it forwards nothing")
	}
}
