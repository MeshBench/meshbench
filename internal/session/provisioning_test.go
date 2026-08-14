package session

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/fixture"
	"github.com/MeshBench/meshbench/internal/scenario"
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
	// What is sent is the session's own settings first - name, clock - and
	// then the regions. The test used to compare against the regions alone,
	// and so failed the moment provisioning learned to set a name.
	want := append(DefaultProvisioning().commandsFor(n), fixture.RegionCommands(n)...)
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

// A node with no regions still gets an answer, because "the panel showed
// nothing" and "this node is told nothing" look identical otherwise.
//
// It is no longer told nothing at all: every node is given its name and the
// run's clock. What it must not be given is a region line, since it carries no
// regions - that is what this guards.
func TestSilentNodeSaysSo(t *testing.T) {
	got := ProvisioningFor(scenario.Node{Name: "Quiet", Kind: scenario.Companion})
	if len(got) < 2 {
		t.Fatalf("expected an explanation, got %v", got)
	}
	for _, l := range got {
		if !l.Comment && strings.HasPrefix(l.Command, "region") {
			t.Errorf("a node with no regions should be told about none, got %q",
				l.Command)
		}
	}
}
