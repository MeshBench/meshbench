package provision_test

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/provision"
)

func hasCmd(cmds []provision.ResolvedCommand, want string) bool {
	for _, c := range cmds {
		if c.Command == want {
			return true
		}
	}
	return false
}

// The example the design is built around: Fife nodes get gb added, and every
// region they already hold - including ones no rule mentioned - survives
// untouched, because nothing asked to change them.
func TestFifeNodesGainGBAndKeepEverythingElse(t *testing.T) {
	ns := provision.NodeState{
		Read: true, Regions: []string{"fif", "sco"},
		DefaultScopeKnown: true, DefaultScope: "sco",
	}
	rules := []provision.Rule{
		{Name: "holds fif", Conditions: []provision.Condition{
			{Field: "regions", Op: "contain", Value: "fif"},
		}, Effects: []provision.Effect{
			{Field: "regions", Mode: provision.ModeAdd, Value: "gb"},
		}},
	}
	got := provision.Resolve(rules, ns)
	if !hasCmd(got, "region put gb") || !hasCmd(got, "region allowf gb") {
		t.Fatalf("gb was not added: %+v", got)
	}
	if hasCmd(got, "region put fif") || hasCmd(got, "region put sco") {
		t.Errorf("regions already held must not be re-put: %+v", got)
	}
}

// A rule that matches but changes nothing sends nothing - reconciliation, not
// scripting.
func TestANodeAlreadyCorrectGetsNoCommands(t *testing.T) {
	ns := provision.NodeState{
		Read: true, Regions: []string{"fif", "gb"},
		Values: map[string]string{"path.hash.mode": "1"}, // wire value for 2 bytes
	}
	rules := []provision.Rule{
		{Effects: []provision.Effect{
			{Field: "regions", Mode: provision.ModeAdd, Value: "gb"},
			{Field: "path.hash.mode", Mode: provision.ModeSet, Value: "2"},
		}},
	}
	got := provision.Resolve(rules, ns)
	if len(got) != 0 {
		t.Fatalf("nothing should have changed, got %+v", got)
	}
}

func TestPathHashIsSentInBytesMinusOneOnTheWire(t *testing.T) {
	ns := provision.NodeState{Read: true, Values: map[string]string{"path.hash.mode": "0"}}
	rules := []provision.Rule{
		{Effects: []provision.Effect{
			{Field: "path.hash.mode", Mode: provision.ModeSet, Value: "3"},
		}},
	}
	got := provision.Resolve(rules, ns)
	if !hasCmd(got, "set path.hash.mode 2") {
		t.Fatalf("3 bytes should be mode 2 on the wire, got %+v", got)
	}
}

// The last matching rule wins for a scalar - regions still union, but a
// scalar effect has exactly one final value and the later rule decides it.
func TestLastMatchingRuleWinsForDefaultScope(t *testing.T) {
	ns := provision.NodeState{Read: true, DefaultScopeKnown: true, DefaultScope: "sco"}
	rules := []provision.Rule{
		{Name: "first", Effects: []provision.Effect{
			{Field: "default-scope", Mode: provision.ModeSet, Value: "gb"},
		}},
		{Name: "second", Effects: []provision.Effect{
			{Field: "default-scope", Mode: provision.ModeSet, Value: "hgh"},
		}},
	}
	got := provision.Resolve(rules, ns)
	if !hasCmd(got, "region default hgh") {
		t.Fatalf("wanted hgh from the later rule, got %+v", got)
	}
	if hasCmd(got, "region default gb") {
		t.Errorf("the earlier rule should have been overridden entirely, got %+v", got)
	}
}

// Un-scoped flood permission must be revocable - an accumulating rule would
// make deny unreachable once anything upstream had allowed it.
func TestUnscopedFloodCanBeRevoked(t *testing.T) {
	ns := provision.NodeState{Read: true, UnscopedFlood: true}
	rules := []provision.Rule{
		{Effects: []provision.Effect{
			{Field: "unscoped-flood", Mode: provision.ModeSet, Value: "drop"},
		}},
	}
	got := provision.Resolve(rules, ns)
	if !hasCmd(got, "region denyf *") {
		t.Fatalf("wanted region denyf *, got %+v", got)
	}
}

func TestUnscopedFloodEffectDoesNotTouchNamedRegions(t *testing.T) {
	ns := provision.NodeState{Read: true, Regions: []string{"fif"}}
	rules := []provision.Rule{
		{Effects: []provision.Effect{
			{Field: "unscoped-flood", Mode: provision.ModeSet, Value: "drop"},
		}},
	}
	got := provision.Resolve(rules, ns)
	for _, c := range got {
		if c.Command == "region put fif" || c.Command == "region allowf fif" {
			t.Errorf("the un-scoped flood effect must not touch named regions: %+v", got)
		}
	}
}

// As imported reproduces what the scenario recorded, not what is currently on
// the wire - and does nothing when the two already agree.
func TestAsImportedReproducesTheScenarioRecord(t *testing.T) {
	ns := provision.NodeState{
		Read: true, Values: map[string]string{"name": "Old Name"},
		Imported: provision.ImportedFacts{Name: "Abernethy Repeater"},
	}
	rules := []provision.Rule{
		{Effects: []provision.Effect{{Field: "name", Mode: provision.ModeAsImported}}},
	}
	got := provision.Resolve(rules, ns)
	if !hasCmd(got, "set name Abernethy Repeater") {
		t.Fatalf("wanted the imported name sent, got %+v", got)
	}
}

func TestAsImportedWithNoImportedValueIsANoOp(t *testing.T) {
	ns := provision.NodeState{Read: true, Values: map[string]string{"cad": "off"}}
	rules := []provision.Rule{
		{Effects: []provision.Effect{{Field: "cad", Mode: provision.ModeAsImported}}},
	}
	got := provision.Resolve(rules, ns)
	if len(got) != 0 {
		t.Fatalf("cad has no import source, wanted nothing sent, got %+v", got)
	}
}

// The clock cannot be diffed against `clock`'s own reply, so it is always
// sent when a rule asks for it - the refusal, if any, belongs in the
// transcript, not silently swallowed here.
func TestClockIsAlwaysSentNotDiffed(t *testing.T) {
	ns := provision.NodeState{Read: true, Values: map[string]string{
		"clock": "14:00 - 30/12/2026 UTC",
	}}
	rules := []provision.Rule{
		{Effects: []provision.Effect{{Field: "clock", Mode: provision.ModeSet, Value: "1788220800"}}},
	}
	got := provision.Resolve(rules, ns)
	if !hasCmd(got, "time 1788220800") {
		t.Fatalf("wanted the clock command sent unconditionally, got %+v", got)
	}
}

func TestACustomEffectIsSentVerbatim(t *testing.T) {
	ns := provision.NodeState{Read: true}
	rules := []provision.Rule{
		{Name: "testing a new build", Effects: []provision.Effect{
			{Mode: provision.ModeSet, CustomSet: "set scope.rotate on"},
		}},
	}
	got := provision.Resolve(rules, ns)
	if len(got) != 1 || got[0].Command != "set scope.rotate on" || got[0].RuleName != "testing a new build" {
		t.Fatalf("got %+v", got)
	}
}

// A companion cannot answer loop.detect, so an effect on it must not send a
// command to a node that has nowhere to put it.
func TestAScalarEffectSkipsACompanionWithNoEquivalent(t *testing.T) {
	ns := provision.NodeState{Read: true, Companion: true}
	rules := []provision.Rule{
		{Effects: []provision.Effect{
			{Field: "loop.detect", Mode: provision.ModeSet, Value: "minimal"},
		}},
	}
	got := provision.Resolve(rules, ns)
	if len(got) != 0 {
		t.Fatalf("loop.detect has no companion equivalent, wanted nothing, got %+v", got)
	}
}

func TestResolveIsOrderIndependentForMatching(t *testing.T) {
	// Two rules that would match each other's effects if conditions evaluated
	// against resolved state rather than the readback - reordering must not
	// change which nodes match.
	ns := provision.NodeState{Read: true, Values: map[string]string{"cad": "off"}}
	a := provision.Rule{Name: "a", Conditions: []provision.Condition{
		{Field: "cad", Op: "is", Value: "off"},
	}, Effects: []provision.Effect{{Field: "cad", Mode: provision.ModeSet, Value: "on"}}}
	b := provision.Rule{Name: "b", Conditions: []provision.Condition{
		{Field: "cad", Op: "is", Value: "on"},
	}, Effects: []provision.Effect{{Field: "cad", Mode: provision.ModeSet, Value: "off"}}}

	forward := provision.Resolve([]provision.Rule{a, b}, ns)
	backward := provision.Resolve([]provision.Rule{b, a}, ns)
	if len(forward) != 1 || forward[0].Command != "set cad on" {
		t.Fatalf("forward: %+v", forward)
	}
	if len(backward) != 1 || backward[0].Command != "set cad on" {
		t.Fatalf("reordering rules changed which one matched: %+v", backward)
	}
}
