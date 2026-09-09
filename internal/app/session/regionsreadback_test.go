package session

import (
	"reflect"
	"testing"
)

// A region set straight over a node's console is read back from its firmware
// with "region list allowed" and "region default", and parsed out of the
// console lines those answer with.
func TestParseRegionReadback(t *testing.T) {
	// The ordinary case: the echoes, the CSV of allowed regions (wildcard
	// first), and the default scope.
	lines := []string{
		"region list allowed",
		"*,sco,fif",
		"region default",
		" default scope is fif",
	}
	regions, scope, err := parseRegionReadback(lines)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(regions, []string{"sco", "fif"}) {
		t.Errorf("regions = %v, want [sco fif] (the wildcard dropped)", regions)
	}
	if scope != "fif" {
		t.Errorf("default scope = %q, want fif", scope)
	}

	// A node that holds no named region answers -none-, and its default is
	// null: an empty list and an empty scope, not an error.
	regions, scope, err = parseRegionReadback([]string{
		"region list allowed", "-none-", "region default", " default scope is <null>",
	})
	if err != nil || len(regions) != 0 || scope != "" {
		t.Errorf("a region-less node parsed to %v %q %v, want [] \"\" nil", regions, scope, err)
	}

	// The hash prefix is stripped from both the list and the default.
	regions, scope, _ = parseRegionReadback([]string{"#sco,#fif", " default scope is #sco"})
	if !reflect.DeepEqual(regions, []string{"sco", "fif"}) || scope != "sco" {
		t.Errorf("hash prefixes not stripped: %v %q", regions, scope)
	}

	// A firmware without region support answers Err, and no list is seen: an
	// error, so the model is not overwritten with a guess.
	if _, _, err := parseRegionReadback([]string{"region list allowed", "Err - ??"}); err == nil {
		t.Error("a node that could not answer should be an error, not an empty region set")
	}

	// Prose in the scrollback is not mistaken for a region list.
	if isRegionCSV("connected to the mesh") {
		t.Error("a sentence was read as a region list")
	}
	if !isRegionCSV("sco,fif") {
		t.Error("a real region list was rejected")
	}
}
