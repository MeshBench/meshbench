package session

import (
	"strings"
	"testing"

	"github.com/A13xB0/meshcoresim/internal/fixture"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// The script has to be the commands, not a description of them. If these ever
// diverge the panel is lying about what the node was sent, which is the exact
// failure this feature exists to prevent.
func TestScriptIsWhatIsSent(t *testing.T) {
	n := scenario.Node{
		Name:    "Alba",
		Kind:    scenario.SimpleRepeater,
		Regions: []string{"sco", "fif"},
	}
	var got []string
	for _, l := range ProvisioningFor(n) {
		if !l.Comment {
			got = append(got, l.Command)
		}
	}
	want := fixture.RegionCommands(n)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("script has drifted from what is sent:\n got %q\nwant %q", got, want)
	}
}

func TestEveryLineSaysWhy(t *testing.T) {
	n := scenario.Node{Name: "Alba", Kind: scenario.SimpleRepeater, Regions: []string{"sco"}}
	for _, l := range ProvisioningFor(n) {
		if strings.TrimSpace(l.Why) == "" {
			t.Errorf("no reason given for %q", l.Command)
		}
	}
}

// A node with nothing to send still gets an answer, because "the panel showed
// nothing" and "this node is told nothing" look identical otherwise.
func TestSilentNodeSaysSo(t *testing.T) {
	got := ProvisioningFor(scenario.Node{Name: "Quiet", Kind: scenario.Companion})
	if len(got) < 2 {
		t.Fatalf("expected an explanation, got %v", got)
	}
	for _, l := range got {
		if !l.Comment {
			t.Errorf("a node with no regions should send nothing, got %q", l.Command)
		}
	}
}
