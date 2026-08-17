package provision_test

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/provision"
)

func TestARuleWithNoConditionsMatchesEverything(t *testing.T) {
	r := provision.Rule{Name: "base"}
	if !r.Matches(provision.NodeState{Name: "unread node"}) {
		t.Fatal("a rule with no conditions must match even before a readback")
	}
}

// The Fife case: default scope is a fact about what a node originates,
// regions held is a fact about what it relays, and a condition on one must
// not accidentally match on the other.
func TestDefaultScopeAndRegionsAreDifferentFacts(t *testing.T) {
	ns := provision.NodeState{
		Read: true, Regions: []string{"fif", "sco"}, DefaultScope: "sco",
		DefaultScopeKnown: true,
	}
	onlyFif := provision.Rule{Conditions: []provision.Condition{
		{Field: "regions", Op: "contain", Value: "fif"},
		{Field: "default-scope", Op: "is not", Value: "sco"},
	}}
	if onlyFif.Matches(ns) {
		t.Error("a node scoped sco should not match 'default scope is not sco'")
	}
	both := provision.Rule{Conditions: []provision.Condition{
		{Field: "regions", Op: "contain", Value: "fif"},
		{Field: "default-scope", Op: "is", Value: "sco"},
	}}
	if !both.Matches(ns) {
		t.Error("a Fife node scoped sco should match both conditions together")
	}
}

func TestAnUnreadNodeMatchesOnlyRulesWithNoConditions(t *testing.T) {
	ns := provision.NodeState{Name: "n"} // Read: false
	withCondition := provision.Rule{Conditions: []provision.Condition{
		{Field: "kind", Op: "is", Value: "simple-repeater"},
	}}
	if withCondition.Matches(ns) {
		t.Error("a condition cannot honestly be evaluated before a readback")
	}
	withoutCondition := provision.Rule{}
	if !withoutCondition.Matches(ns) {
		t.Error("a rule with no conditions needs no readback")
	}
}

func TestPathHashConditionSkipsACompanionRatherThanGuessing(t *testing.T) {
	// loop.detect has no companion equivalent (CompanionField == "").
	ns := provision.NodeState{Read: true, Companion: true, Values: map[string]string{
		"loop.detect": "off",
	}}
	r := provision.Rule{Conditions: []provision.Condition{
		{Field: "loop.detect", Op: "is", Value: "off"},
	}}
	if r.Matches(ns) {
		t.Error("a companion cannot answer loop.detect, so the condition must not match")
	}
}

func TestAreaConditionReadsInsideAndOutside(t *testing.T) {
	ns := provision.NodeState{Read: true, Areas: []string{"Cairngorms National Park"}}
	inside := provision.Rule{Conditions: []provision.Condition{
		{Field: "area", Op: "inside", Value: "Cairngorms National Park"},
	}}
	outside := provision.Rule{Conditions: []provision.Condition{
		{Field: "area", Op: "outside", Value: "Fife group coverage"},
	}}
	if !inside.Matches(ns) {
		t.Error("node in Cairngorms NP should match 'inside Cairngorms NP'")
	}
	if !outside.Matches(ns) {
		t.Error("node not in Fife should match 'outside Fife group coverage'")
	}
}

func TestCustomConditionMatchesAgainstItsOwnReadback(t *testing.T) {
	ns := provision.NodeState{Read: true, Values: map[string]string{
		"custom:get scope.rotate": "on",
	}}
	r := provision.Rule{Conditions: []provision.Condition{
		{Custom: true, CustomGet: "get scope.rotate", Op: "is not", Value: "on"},
	}}
	if r.Matches(ns) {
		t.Error("a node answering 'on' must not match 'is not on'")
	}
}

func TestRequiredReadsIsTheUnionAcrossRules(t *testing.T) {
	rules := []provision.Rule{
		{Conditions: []provision.Condition{{Field: "path.hash.mode", Op: "is", Value: "1"}}},
		{Effects: []provision.Effect{{Field: "loop.detect", Mode: provision.ModeSet, Value: "minimal"}}},
		{Conditions: []provision.Condition{{Custom: true, CustomGet: "get scope.rotate"}}},
	}
	got := provision.RequiredReads(rules)
	want := map[string]bool{
		"region": true, "region default": true,
		"get path.hash.mode": true, "get loop.detect": true,
		"get scope.rotate": true,
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, wanted the %d commands in %v", got, len(want), want)
	}
	for _, g := range got {
		if !want[g] {
			t.Errorf("unexpected read command %q", g)
		}
	}
}

func TestRequiredReadsNeverIncludesAnUnreadableKey(t *testing.T) {
	rules := []provision.Rule{
		{Effects: []provision.Effect{{Field: "owner.info", Mode: provision.ModeSet, Value: "x"}}},
	}
	for _, g := range provision.RequiredReads(rules) {
		if g == "get owner.info" {
			t.Fatal("owner.info has no Get command and must not be requested")
		}
	}
}
