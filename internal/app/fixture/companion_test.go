package fixture

import (
	"strings"
	"testing"
	"time"

	embedded "github.com/MeshBench/meshbench/fixtures"
	"github.com/MeshBench/meshbench/internal/mesh/proto"
	"github.com/MeshBench/meshbench/internal/world/provider"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Every scheduled send must be sayable at the node it names.
//
// fixture-fife-strict shipped for months failing its own assertion because its
// three `public hello` lines were aimed at companions and sent as console text.
// Nothing rejected them: writing at a port succeeds whether or not anything is
// reading it, so the run reported the sends, delivered nothing, and looked like
// broken RF. This is the check that a fixture's schedule is one its nodes can
// actually take, and it needs no firmware to make it.
func TestEveryShippedScheduleIsSayableAtItsNode(t *testing.T) {
	names, err := embedded.FS.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the embedded fixtures: %v", err)
	}
	seen := 0
	for _, entry := range names {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		fx, err := Load(entry.Name())
		if err != nil {
			t.Fatalf("%s: %v", entry.Name(), err)
		}
		for _, snd := range fx.Sends {
			node, ok := nodeNamed(fx.Nodes, snd.Node)
			if !ok {
				t.Errorf("%s: a send names %q, which is not a node in it",
					entry.Name(), snd.Node)
				continue
			}
			seen++
			if !SpeaksCompanion(node.Kind) {
				continue
			}
			if _, err := CompanionCommand(snd.Command, time.Unix(0, 0)); err != nil {
				t.Errorf("%s: %s is a %s, and %q is not something one can be told: %v",
					entry.Name(), snd.Node, node.Kind, snd.Command, err)
			}
		}
	}
	if seen == 0 {
		t.Fatal("no shipped fixture schedules anything, so this checked nothing")
	}
}

// A companion sends under a scope or it sends into a void.
//
// Unscoped traffic is carried by a different set of repeaters - in a strictly
// scoped mesh, by none of them - and nothing anywhere reports an error: the
// message is originated, every repeater computes a different key, and declines.
func TestAScopedCompanionIsGivenItsScopeKey(t *testing.T) {
	n := scenario.Node{
		Name: "Jazzy", Kind: scenario.Companion, DefaultScope: "#sco",
		TxPowerDBm: 22,
		Radio: scenario.RadioConfig{
			CentreHz: 869.618e6, BandwidthHz: 62500, SpreadFactor: 8, CodingRate: 1,
		},
	}
	want := proto.SetDefaultScope("#sco", provider.RegionKey("#sco"))
	if !containsFrame(CompanionProvisioning(n, 0), want) {
		t.Error("a companion with a default scope was not told its scope key")
	}

	// The bare spelling is the one a repeater's own CLI takes, and deriving the
	// key from it produces a key every repeater disagrees with.
	n.DefaultScope = "sco"
	if !containsFrame(CompanionProvisioning(n, 0), want) {
		t.Error("a scope written without its # produced a different key")
	}

	n.DefaultScope = ""
	for _, f := range CompanionProvisioning(n, 0) {
		if len(f) > 0 && proto.Command(f[0]) == proto.CmdSetFloodScopeKey {
			t.Error("a companion with no scope was given one anyway")
		}
	}
}

// An unknown word is refused rather than swallowed, which is the whole lesson.
func TestAnUnknownScheduledCommandIsRefused(t *testing.T) {
	if _, err := CompanionCommand("reboot now", time.Unix(0, 0)); err == nil {
		t.Error("a command no companion has was accepted")
	}
	if _, err := CompanionCommand("public", time.Unix(0, 0)); err == nil {
		t.Error("public with no message was accepted")
	}
	if _, err := CompanionCommand("public hello there", time.Unix(0, 0)); err != nil {
		t.Errorf("public with a message was refused: %v", err)
	}
}

func nodeNamed(nodes []scenario.Node, name string) (scenario.Node, bool) {
	for _, n := range nodes {
		if n.Name == name {
			return n, true
		}
	}
	return scenario.Node{}, false
}

func containsFrame(frames [][]byte, want []byte) bool {
	for _, f := range frames {
		if string(f) == string(want) {
			return true
		}
	}
	return false
}
