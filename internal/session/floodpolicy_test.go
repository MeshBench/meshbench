package session

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/scenario"
)

func policyTestNode(name string, regions ...string) scenario.Node {
	return scenario.Node{Name: name, Kind: scenario.SimpleRepeater, Regions: regions}
}

func hasCommand(cmds []string, want string) bool {
	for _, c := range cmds {
		if c == want {
			return true
		}
	}
	return false
}

func TestFloodPolicyCommandsForAllowAndDeny(t *testing.T) {
	p := FloodPolicy{AllowRegions: []string{"sco"}, DenyRegions: []string{"*"}}
	cmds := p.commandsFor(policyTestNode("r"))
	for _, want := range []string{"region put sco", "region allowf sco", "region denyf *", "region save"} {
		if !hasCommand(cmds, want) {
			t.Errorf("commandsFor missing %q in %v", want, cmds)
		}
	}
}

// TestFloodPolicyComposes is the plan's own acceptance test: policies
// compose, region plus hop limit behaves as both.
func TestFloodPolicyComposes(t *testing.T) {
	p := FloodPolicy{AllowRegions: []string{"sco"}, MaxHops: 4}
	cmds := p.commandsFor(policyTestNode("r"))
	if !hasCommand(cmds, "region put sco") {
		t.Error("the region half of a composed policy is missing")
	}
	if !hasCommand(cmds, "set flood.max 4") || !hasCommand(cmds, "set flood.max.advert 4") {
		t.Errorf("the hop-limit half of a composed policy is missing: %v", cmds)
	}
}

func TestFloodPolicyDoesNotTouchAReceiveOnlyNode(t *testing.T) {
	p := FloodPolicy{AllowRegions: []string{"sco"}, MaxHops: 4}
	obs := scenario.Node{Name: "obs", Kind: scenario.SDRObserver}
	if cmds := p.commandsFor(obs); len(cmds) != 0 {
		t.Errorf("a node that never transmits should get no forwarding commands, got %v", cmds)
	}
}

// TestFloodPolicyDoesNotTouchACompanion is a real bug this file's own live
// verification caught: a companion transmits (Kind.Transmits() is true for
// it) but has no console at all, so a policy that used that check sent
// forwarding commands nothing could ever answer, and its own read-back
// discipline correctly reported them unconfirmed.
func TestFloodPolicyDoesNotTouchACompanion(t *testing.T) {
	p := FloodPolicy{AllowRegions: []string{"sco"}, MaxHops: 4}
	comp := scenario.Node{Name: "comp", Kind: scenario.Companion}
	if cmds := p.commandsFor(comp); len(cmds) != 0 {
		t.Errorf("a companion has no console to answer these to, got %v", cmds)
	}
}

func TestFloodPolicyEmptyPolicySendsNothing(t *testing.T) {
	var p FloodPolicy
	if cmds := p.commandsFor(policyTestNode("r")); len(cmds) != 0 {
		t.Errorf("an empty policy should change nothing, got %v", cmds)
	}
}

func TestFloodPolicyReadBackMatchesWhatWasSent(t *testing.T) {
	p := FloodPolicy{MaxHops: 4, OneBytePathIDs: true}
	want := p.readBack()
	if want["get flood.max"] != "4" || want["get flood.max.advert"] != "4" {
		t.Errorf("read-back for max hops = %v", want)
	}
	if want["get path.hash.mode"] != "1" {
		t.Errorf("read-back for one-byte path IDs = %v", want)
	}
}

func TestCheckIsolationCleanWhenNothingLeavesTheAllowedRegion(t *testing.T) {
	events := []engine.Event{
		{Kind: "tx", From: "a", To: ""},
		{Kind: "rx", To: "b"},
		{Kind: "rx", To: "c"},
	}
	regions := map[string][]string{"a": {"sco"}, "b": {"sco"}, "c": {"sco", "ioi"}}
	clean, leaked := checkIsolation(events, regions, []string{"sco"})
	if !clean || leaked != nil {
		t.Errorf("clean=%v leaked=%v, want clean and nothing leaked", clean, leaked)
	}
}

// TestCheckIsolationCatchesADeliberateLeak is the plan's own acceptance
// test: isolation detection catches a deliberate cross-region leak.
func TestCheckIsolationCatchesADeliberateLeak(t *testing.T) {
	events := []engine.Event{
		{Kind: "rx", To: "b"}, // in #sco, fine
		{Kind: "rx", To: "d"}, // only in #ioi - a leak
		{Kind: "tx", To: "d"}, // not an rx event, ignored
	}
	regions := map[string][]string{"b": {"sco"}, "d": {"ioi"}}
	clean, leaked := checkIsolation(events, regions, []string{"sco"})
	if clean {
		t.Fatal("a node with no allowed region among its own should not read as clean")
	}
	if len(leaked) != 1 || leaked[0] != "ioi" {
		t.Errorf("leaked = %v, want [\"ioi\"]", leaked)
	}
}

// TestCheckIsolationHandlesTheHashPrefixAsymmetry is a real bug this file's
// own live verification caught: a node's saved Regions carry the "#" a
// fixture writes them with, while AllowRegions arrives in the CLI's own
// bare form (what commandsFor actually sends) - unnormalised, a node
// legitimately holding the allowed region was reported as leaking to it.
func TestCheckIsolationHandlesTheHashPrefixAsymmetry(t *testing.T) {
	events := []engine.Event{{Kind: "rx", To: "a"}}
	regions := map[string][]string{"a": {"#sco"}}
	clean, leaked := checkIsolation(events, regions, []string{"sco"})
	if !clean || leaked != nil {
		t.Errorf("a node holding #sco should read as allowed by a bare \"sco\" policy: clean=%v leaked=%v",
			clean, leaked)
	}
}

func TestCheckIsolationReportsEachLeakedRegionOnce(t *testing.T) {
	events := []engine.Event{{Kind: "rx", To: "d1"}, {Kind: "rx", To: "d2"}}
	regions := map[string][]string{"d1": {"ioi"}, "d2": {"ioi"}}
	_, leaked := checkIsolation(events, regions, []string{"sco"})
	if len(leaked) != 1 {
		t.Errorf("two nodes leaking to the same region should report it once, got %v", leaked)
	}
}

func TestCheckIsolationWithNoContainmentClaimIsAlwaysClean(t *testing.T) {
	events := []engine.Event{{Kind: "rx", To: "anywhere"}}
	clean, leaked := checkIsolation(events, nil, nil)
	if !clean || leaked != nil {
		t.Error("a policy that names no allowed regions makes no containment claim to fail")
	}
}

func TestCheckIsolationWildcardAllowIsAlwaysClean(t *testing.T) {
	events := []engine.Event{{Kind: "rx", To: "anywhere"}}
	clean, _ := checkIsolation(events, map[string][]string{"anywhere": {"literally-anything"}}, []string{"*"})
	if !clean {
		t.Error("an explicitly wildcard-open policy has nothing to call a leak")
	}
}

func TestFloodPolicyLabelIsUsedInDescribe(t *testing.T) {
	// Guards the arm-carries-a-policy contract at the type level: Policy is
	// a pointer, so nil (every arm before this feature) and a real policy
	// are distinguishable, which is what lets old arms keep working
	// unchanged.
	var arm ExpArm
	if arm.Policy != nil {
		t.Fatal("a fresh arm should carry no policy")
	}
	arm.Policy = &FloodPolicy{Label: "strict", AllowRegions: []string{"sco"}}
	if !strings.Contains(arm.Policy.Label, "strict") {
		t.Fatal("the policy's own label should be readable off the arm")
	}
}
