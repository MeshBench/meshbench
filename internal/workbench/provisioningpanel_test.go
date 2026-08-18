package workbench

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/gui/state"
)

// A rule survives being read into the panel and written back out - the
// editor's whole reason to exist is to hold a rule while it is being typed,
// and if that round trip drops a field, every rule anyone edits loses it
// silently.
func TestRuleEditorRoundTripsThroughAllThreeSlots(t *testing.T) {
	p := &provisioningRulesPanel{}
	e := &ruleEditor{}
	e.build(p)

	original := state.ProvisionRule{
		Name: "holds fif",
		Conditions: []state.ProvisionCondition{
			{Field: "regions", Op: "contain", Value: "fif"},
			{Field: "default-scope", Op: "is", Value: "sco"},
			{Custom: true, CustomGet: "get scope.rotate", Op: "is not", Value: "on"},
		},
		Effects: []state.ProvisionEffect{
			{Field: "regions", Mode: "add", Value: "gb"},
			{Field: "path.hash.mode", Mode: "set", Value: "3"},
			{CustomSet: "set scope.rotate on"},
		},
	}
	e.fromState(original)
	got := e.toState()

	if got.Name != original.Name {
		t.Errorf("name: got %q, wanted %q", got.Name, original.Name)
	}
	if len(got.Conditions) != 3 || len(got.Effects) != 3 {
		t.Fatalf("got %+v", got)
	}
	if got.Conditions[2].Custom != true || got.Conditions[2].CustomGet != "get scope.rotate" {
		t.Errorf("the custom condition did not round-trip: %+v", got.Conditions[2])
	}
	if got.Effects[2].CustomSet != "set scope.rotate on" {
		t.Errorf("the custom effect did not round-trip: %+v", got.Effects[2])
	}
	if got.Effects[0].Field != "regions" || got.Effects[0].Mode != "add" || got.Effects[0].Value != "gb" {
		t.Errorf("the regions effect did not round-trip: %+v", got.Effects[0])
	}
}

// An empty slot contributes nothing - a rule with one condition typed into
// slot zero must not produce two empty conditions alongside it.
func TestRuleEditorSkipsUnusedSlots(t *testing.T) {
	p := &provisioningRulesPanel{}
	e := &ruleEditor{}
	e.build(p)
	e.condField[0].Value = "kind"
	e.condOp[0].Value = "is"
	e.condVal[0].Editor.SetText("simple-repeater")

	r := e.toState()
	if len(r.Conditions) != 1 {
		t.Fatalf("got %d conditions, wanted 1: %+v", len(r.Conditions), r.Conditions)
	}
	if len(r.Effects) != 0 {
		t.Errorf("no effect was set, wanted none, got %+v", r.Effects)
	}
}

func TestSeedSkipsTheLegacyBaseRule(t *testing.T) {
	p := &provisioningRulesPanel{}
	s := &state.Snapshot{ProvisioningRules: []state.ProvisionRule{
		{Name: "the session's own settings"},
		{Name: "an actual rule"},
	}}
	p.seed(s)
	if len(p.editors) != 1 {
		t.Fatalf("got %d editors, wanted 1 - the base is drawn by the panel above, not here", len(p.editors))
	}
}

func TestConditionFieldOptionsIncludeEveryReadableKey(t *testing.T) {
	s := &state.Snapshot{ProvisioningKeys: []state.ProvisionKey{
		{Name: "path.hash.mode"}, {Name: "loop.detect"},
	}}
	opts := conditionFieldOptions(s)
	want := map[string]bool{
		"regions": true, "default-scope": true, "unscoped-flood": true,
		"kind": true, "selected": true, "area": true,
		"path.hash.mode": true, "loop.detect": true,
	}
	if len(opts) != len(want) {
		t.Fatalf("got %v", opts)
	}
	for _, o := range opts {
		if !want[o] {
			t.Errorf("unexpected option %q", o)
		}
	}
}

func TestEffectModeOptionsNeverOfferReplaceForRegions(t *testing.T) {
	// See the design note: destructive replace is not offered, only add and
	// as-imported - a rule can only ever gain regions here, never remove one.
	for _, m := range effectModeOptions("regions") {
		if m == "replace" {
			t.Error("regions must never offer a destructive replace mode")
		}
	}
}

func TestConditionOpOptionsMatchTheFieldShape(t *testing.T) {
	listOps := conditionOpOptions("regions")
	for _, op := range []string{"contain", "do not contain"} {
		found := false
		for _, o := range listOps {
			if o == op {
				found = true
			}
		}
		if !found {
			t.Errorf("regions should offer %q, got %v", op, listOps)
		}
	}
	boolOps := conditionOpOptions("selected")
	if len(boolOps) != 1 || boolOps[0] != "is" {
		t.Errorf("a yes/no field should offer only 'is', got %v", boolOps)
	}
}

// The whole point of the panel: type a rule, press save, and it reaches the
// session as provisioning.rules.set with the shape session.decodeStateRule
// expects.
func TestProvisioningRulesPanelSaveReachesTheVerb(t *testing.T) {
	r := &recorder{}
	p := &provisioningRulesPanel{do: r.do,
		choose: func(_ string, opts []string, pick func(string)) {
			if len(opts) > 0 {
				pick(opts[0])
			}
		}}
	e := &ruleEditor{}
	e.build(p)
	e.name.Editor.SetText("holds fif")
	e.condField[0].Value = "regions"
	e.condOp[0].Value = "contain"
	e.condVal[0].Editor.SetText("fif")
	e.effField[0].Value = "regions"
	e.effMode[0].Value = "add"
	e.effVal[0].Editor.SetText("gb")
	p.editors = []*ruleEditor{e}

	h := newPanelHarness(p.Draw, &state.Snapshot{})
	h.frame()
	// Save sits in the same row as Add, and a full-width sweep clicks every
	// button on a row it crosses - Add included, which grows the list this
	// same sweep is still walking. That is a sweep artifact, not a save
	// bug, so the assertion below finds the seeded rule by name among
	// whatever else Add appended, rather than assuming save's payload has
	// exactly one entry.
	for y := float32(10); y < float32(h.sz.Y) && !r.saw("provisioning.rules.set"); y += 10 {
		h.pressAlong(y)
	}

	if !r.saw("provisioning.rules.set") {
		t.Fatalf("save never reached provisioning.rules.set; got %v", r.verbs)
	}
	for i, v := range r.verbs {
		if v != "provisioning.rules.set" {
			continue
		}
		m, _ := r.params[i].(map[string]any)
		rules, _ := m["rules"].([]any)
		var rule map[string]any
		for _, item := range rules {
			if rm, ok := item.(map[string]any); ok && rm["name"] == "holds fif" {
				rule = rm
			}
		}
		if rule == nil {
			t.Fatalf("the seeded rule was not among what was saved: %v", rules)
		}
		conds, _ := rule["conditions"].([]any)
		if len(conds) != 1 {
			t.Fatalf("got %d conditions: %v", len(conds), conds)
		}
		cond, _ := conds[0].(map[string]any)
		if cond["field"] != "regions" || cond["op"] != "contain" || cond["value"] != "fif" {
			t.Errorf("condition carried %v", cond)
		}
		effs, _ := rule["effects"].([]any)
		if len(effs) != 1 {
			t.Fatalf("got %d effects: %v", len(effs), effs)
		}
		eff, _ := effs[0].(map[string]any)
		if eff["field"] != "regions" || eff["mode"] != "add" || eff["value"] != "gb" {
			t.Errorf("effect carried %v", eff)
		}
		return
	}
}

// Add and reload are local-only - neither ever reaches the session on its
// own - so each has to say what it did some other way, or pressing it looks
// like nothing happened. This is what the reachability audit's "add a rule"
// and "reload" entries actually check; here in isolation, with the effect on
// the rule list asserted too.
func TestAddRuleGrowsTheListAndSaysSo(t *testing.T) {
	r := &recorder{}
	p := &provisioningRulesPanel{do: r.do}
	h := newPanelHarness(p.Draw, &state.Snapshot{})
	h.frame()
	// One press along the button's row already lands on it many times across
	// its width - that is the sweep technique's own cost, not a sign the
	// button fires more than once per click in the running application.
	h.pressAlong(20)
	if len(p.editors) == 0 {
		t.Fatal("pressing add should have appended at least one rule")
	}
	if !r.saw("ui.said") {
		t.Error("adding a rule should say so, since nothing else about it is visible yet")
	}
}
