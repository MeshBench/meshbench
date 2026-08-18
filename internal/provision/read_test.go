package provision_test

import (
	"reflect"
	"testing"

	"github.com/MeshBench/meshbench/internal/provision"
)

func TestParseGetReplyStripsThePrefix(t *testing.T) {
	v, ok := provision.ParseGetReply("> minimal")
	if !ok || v != "minimal" {
		t.Fatalf("got %q, %v", v, ok)
	}
	if _, ok := provision.ParseGetReply("Unknown command"); ok {
		t.Error("a reply with no '> ' prefix is not a get reply")
	}
}

// The example from RegionMap.cpp's own printChildRegions: the wildcard's own
// line, then children indented, "^" on home, "F" only where flood is
// allowed.
func TestParseRegionTreeReadsTheFirmwareFormat(t *testing.T) {
	lines := []string{
		"* F",
		" Europe F",
		"  UK^ F",
		"  FR",
		" NZ",
	}
	regions, unscoped := provision.ParseRegionTree(lines)
	if !unscoped {
		t.Error("the wildcard line was 'F' - un-scoped floods should read as allowed")
	}
	want := []string{"Europe", "UK"}
	if !reflect.DeepEqual(regions, want) {
		t.Errorf("got %v, wanted %v - FR and NZ are not flood-allowed and NZ is denied", regions, want)
	}
}

func TestParseRegionTreeStripsTheHashtagPrefix(t *testing.T) {
	regions, _ := provision.ParseRegionTree([]string{"* ", " sco F"})
	if !reflect.DeepEqual(regions, []string{"sco"}) {
		t.Errorf("got %v", regions)
	}
}

func TestParseRegionDefaultDistinguishesUnscopedFromUnread(t *testing.T) {
	scope, known := provision.ParseRegionDefault("default scope is sco")
	if !known || scope != "sco" {
		t.Fatalf("got %q, %v", scope, known)
	}
	scope, known = provision.ParseRegionDefault("default scope is <null>")
	if !known || scope != "" {
		t.Fatalf("<null> should read as known-and-unscoped, got %q, %v", scope, known)
	}
	if _, known := provision.ParseRegionDefault("Unknown command"); known {
		t.Error("a reply that is not this format must not read as known")
	}
}

func TestRequiredReadsAlwaysIncludesRegionsAndDefaultScope(t *testing.T) {
	got := provision.RequiredReads(nil)
	want := map[string]bool{"region": true, "region default": true}
	if len(got) != 2 || !want[got[0]] || !want[got[1]] {
		t.Fatalf("got %v", got)
	}
}
