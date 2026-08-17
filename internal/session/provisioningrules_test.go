package session

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/provision"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// The base is a rule like any other, expressed from the legacy flat settings.
// This is what makes "a rule with no conditions matches everything" the
// mechanism a plain default run uses, rather than a separate concept.
func TestLegacyAsRuleHasNoConditions(t *testing.T) {
	r := legacyAsRule(DefaultProvisioning())
	if len(r.Conditions) != 0 {
		t.Fatalf("the legacy base must match everything, got conditions %+v", r.Conditions)
	}
}

func TestLegacyAsRuleCarriesTheNewDefaults(t *testing.T) {
	r := legacyAsRule(DefaultProvisioning())
	want := map[string]string{"path.hash.mode": "3", "loop.detect": "minimal"}
	for _, e := range r.Effects {
		if v, ok := want[e.Field]; ok {
			if e.Value != v {
				t.Errorf("%s: got %q, wanted %q", e.Field, e.Value, v)
			}
			delete(want, e.Field)
		}
	}
	if len(want) != 0 {
		t.Errorf("missing effects: %v", want)
	}
}

// The legacy field stores the wire value (mode 2 = 3 bytes); rules speak
// bytes. Losing that +1 anywhere in the translation would silently send the
// wrong path hash size.
func TestLegacyAsRuleTranslatesPathHashModeToBytes(t *testing.T) {
	p := DefaultProvisioning()
	p.PathHashMode = 0 // mode 0 = 1 byte
	r := legacyAsRule(p)
	for _, e := range r.Effects {
		if e.Field == "path.hash.mode" && e.Value != "1" {
			t.Errorf("mode 0 should read as 1 byte, got %q", e.Value)
		}
	}
}

func TestLegacyAsRuleOmitsWhatWasNotAsked(t *testing.T) {
	// PathHashMode's own "leave alone" sentinel is -1, not the zero value -
	// 0 is a real, meaningful wire value (one-byte hashes) - so an all-zero
	// literal is not the same thing as "nothing was asked for".
	r := legacyAsRule(Provisioning{PathHashMode: -1, CompPathHashMode: -1})
	if len(r.Effects) != 0 {
		t.Errorf("nothing was asked for, wanted no effects, got %+v", r.Effects)
	}
}

func TestActiveRulesPutsTheBaseFirst(t *testing.T) {
	s := &Sim{rules: []provision.Rule{{Name: "an override"}}}
	got := s.activeRules()
	if len(got) != 2 || got[1].Name != "an override" {
		t.Fatalf("got %+v", got)
	}
}

// A rule can override the base's own effects, the same as any two study
// rules can override each other - there is nothing special about rule zero
// once it exists.
func TestAnOverrideRuleCanChangeWhatTheBaseSet(t *testing.T) {
	p := DefaultProvisioning() // path.hash.mode 2 (3 bytes)
	s := &Sim{prov: &p, rules: []provision.Rule{
		{Name: "testing 1 byte", Effects: []provision.Effect{
			{Field: "path.hash.mode", Mode: provision.ModeSet, Value: "1"},
		}},
	}}
	ns := provision.NodeState{Read: true, Values: map[string]string{"path.hash.mode": "2"}}
	cmds := provision.Resolve(s.activeRules(), ns)
	found := false
	for _, c := range cmds {
		if c.Command == "set path.hash.mode 0" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the override should have won, got %+v", cmds)
	}
}

func TestParseReadbackFillsInWhatCameBack(t *testing.T) {
	n := repeaterNode("Abernethy Repeater")
	entries := []transcriptEntry{
		{Command: "get name", Reply: []string{"> Abernethy Repeater"}},
		{Command: "get path.hash.mode", Reply: []string{"> 0"}},
		{Command: "region", Reply: []string{"* F", " sco F"}},
		{Command: "region default", Reply: []string{"default scope is sco"}},
	}
	ns := parseReadback(n, entries, []string{"Cairngorms National Park"}, true)
	if !ns.Read {
		t.Fatal("Read must be true once a readback has run")
	}
	if ns.Values["name"] != "Abernethy Repeater" || ns.Values["path.hash.mode"] != "0" {
		t.Errorf("values: %+v", ns.Values)
	}
	if len(ns.Regions) != 1 || ns.Regions[0] != "sco" {
		t.Errorf("regions: %v", ns.Regions)
	}
	if ns.DefaultScope != "sco" || !ns.DefaultScopeKnown {
		t.Errorf("default scope: %q known=%v", ns.DefaultScope, ns.DefaultScopeKnown)
	}
	if !ns.Selected || len(ns.Areas) != 1 || ns.Areas[0] != "Cairngorms National Park" {
		t.Errorf("selected=%v areas=%v", ns.Selected, ns.Areas)
	}
	if ns.Imported.Name != n.Name {
		t.Errorf("imported facts should carry the scenario's own name, got %+v", ns.Imported)
	}
}

// A field the firmware did not answer must stay unset, not read as empty or
// zero - a companion or an old firmware that cannot answer must not look
// like one that answered "off".
func TestParseReadbackLeavesUnansweredFieldsUnset(t *testing.T) {
	n := repeaterNode("N")
	ns := parseReadback(n, nil, nil, false)
	if _, ok := ns.Values["loop.detect"]; ok {
		t.Error("a field with no reply must not appear in Values at all")
	}
	if ns.DefaultScopeKnown {
		t.Error("default scope was never read, so it must not read as known")
	}
}

func TestParseReadbackKeepsACustomConditionsAnswerNamespaced(t *testing.T) {
	n := repeaterNode("N")
	entries := []transcriptEntry{{Command: "get scope.rotate", Reply: []string{"> on"}}}
	ns := parseReadback(n, entries, nil, false)
	// "get scope.rotate" is not in provision.Table, so it is stored under the
	// custom: namespace rather than as though it were a known key.
	if v, ok := ns.Values["custom:get scope.rotate"]; !ok || v != "on" {
		t.Errorf("values: %+v", ns.Values)
	}
	if _, ok := ns.Values["scope.rotate"]; ok {
		t.Error("an unknown key must not be stored as though it were in the table")
	}
}

func TestDecodeStateRuleReadsAConditionAndAnEffect(t *testing.T) {
	item := map[string]any{
		"name": "holds fif",
		"conditions": []any{
			map[string]any{"field": "regions", "op": "contain", "value": "fif"},
		},
		"effects": []any{
			map[string]any{"field": "regions", "mode": "add", "value": "gb"},
		},
	}
	r, err := decodeStateRule(item)
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "holds fif" || len(r.Conditions) != 1 || len(r.Effects) != 1 {
		t.Fatalf("got %+v", r)
	}
	if r.Conditions[0].Field != "regions" || r.Conditions[0].Value != "fif" {
		t.Errorf("condition: %+v", r.Conditions[0])
	}
	if r.Effects[0].Mode != "add" || r.Effects[0].Value != "gb" {
		t.Errorf("effect: %+v", r.Effects[0])
	}
}

func TestDecodeStateRuleRejectsSomethingThatIsNotAnObject(t *testing.T) {
	if _, err := decodeStateRule("not a rule"); err == nil {
		t.Error("a non-object item should be refused, not silently ignored")
	}
}

func TestToAndFromStateRuleRoundTrip(t *testing.T) {
	r := provision.Rule{
		Name: "test",
		Conditions: []provision.Condition{
			{Field: "kind", Op: "is", Value: "simple-repeater"},
			{Custom: true, CustomGet: "get scope.rotate", Op: "is", Value: "on"},
		},
		Effects: []provision.Effect{
			{Field: "loop.detect", Mode: provision.ModeSet, Value: "minimal"},
			{Mode: provision.ModeSet, CustomSet: "set scope.rotate on"},
		},
	}
	got := fromStateRule(toStateRule(r))
	if got.Name != r.Name || len(got.Conditions) != 2 || len(got.Effects) != 2 {
		t.Fatalf("got %+v", got)
	}
	if !got.Conditions[1].Custom || got.Conditions[1].CustomGet != "get scope.rotate" {
		t.Errorf("custom condition lost in the round trip: %+v", got.Conditions[1])
	}
	if got.Effects[1].CustomSet != "set scope.rotate on" {
		t.Errorf("custom effect lost in the round trip: %+v", got.Effects[1])
	}
}

// A study can accept a place made of several rings (a multipolygon, or two
// separate accepted searches sharing a name) - areasContaining must test the
// union of all of them under one area name, not just the first.
func TestAreasContainingGroupsMultiRingBoundariesByName(t *testing.T) {
	square := func(clat, clon float64) scenario.Ring {
		return scenario.Ring{
			{Lat: clat - 1, Lon: clon - 1}, {Lat: clat - 1, Lon: clon + 1},
			{Lat: clat + 1, Lon: clon + 1}, {Lat: clat + 1, Lon: clon - 1},
		}
	}
	areas := []scenario.Boundary{
		{Name: "Split Region", Rings: []scenario.Ring{square(0, 0)}},
		{Name: "Split Region", Rings: []scenario.Ring{square(10, 10)}},
		{Name: "Elsewhere", Rings: []scenario.Ring{square(50, 50)}},
	}
	got := areasContaining(areas, scenario.LatLon{Lat: 10, Lon: 10})
	if len(got) != 1 || got[0] != "Split Region" {
		t.Fatalf("a point in the second ring of a split boundary should still "+
			"match its name, got %v", got)
	}
	if got := areasContaining(areas, scenario.LatLon{Lat: 90, Lon: 90}); got != nil {
		t.Errorf("a point in no boundary should match nothing, got %v", got)
	}
}
