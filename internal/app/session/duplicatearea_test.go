package session

import (
	"os"
	"path/filepath"
	"testing"
)

// A fixture that names the same place twice is corrected on the way in.
//
// The shipped files were fixed, but a fixture somebody saved before
// boundary.accept learned to refuse a duplicate is still out there, and opening
// one used to put the duplicate back into the world past that guard. Correcting
// it here means the rule holds however the areas arrived.
func TestLoadingAFixtureKeepsEachStudyAreaOnce(t *testing.T) {
	const twice = `{
	  "format": 1,
	  "name": "doubled",
	  "nodes": [{"Name": "R1", "Kind": "simple-repeater", "Position": {"Lat": 56.25, "Lon": -3.1}}],
	  "areas": [
	    {"name": "Fife", "boundaries": [{"Rings": [[{"Lat": 56.2, "Lon": -3.2}, {"Lat": 56.3, "Lon": -3.2}, {"Lat": 56.3, "Lon": -3.0}]]}]},
	    {"name": "Fife", "boundaries": [{"Rings": [[{"Lat": 56.2, "Lon": -3.2}, {"Lat": 56.3, "Lon": -3.2}, {"Lat": 56.3, "Lon": -3.0}]]}]}
	  ]
	}`
	p := filepath.Join(t.TempDir(), "doubled.json")
	if err := os.WriteFile(p, []byte(twice), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := LoadFixture(p)
	if err != nil {
		t.Fatalf("the fixture did not load: %v", err)
	}
	if len(got.areas) != 1 {
		t.Fatalf("the same place was kept %d times, so everything that walks "+
			"the areas walks it twice", len(got.areas))
	}
	if got.areas[0].Name != "Fife" {
		t.Errorf("the wrong area survived: %q", got.areas[0].Name)
	}
	if len(got.areas[0].Rings) == 0 {
		t.Error("the surviving area lost its geometry")
	}
}

// Two different places both stay.
func TestLoadingAFixtureKeepsPlacesThatDiffer(t *testing.T) {
	const two = `{
	  "format": 1,
	  "name": "two",
	  "nodes": [{"Name": "R1", "Kind": "simple-repeater", "Position": {"Lat": 56.25, "Lon": -3.1}}],
	  "areas": [
	    {"name": "Fife", "boundaries": [{"Rings": [[{"Lat": 56.2, "Lon": -3.2}, {"Lat": 56.3, "Lon": -3.2}, {"Lat": 56.3, "Lon": -3.0}]]}]},
	    {"name": "Angus", "boundaries": [{"Rings": [[{"Lat": 56.6, "Lon": -2.9}, {"Lat": 56.7, "Lon": -2.9}, {"Lat": 56.7, "Lon": -2.7}]]}]}
	  ]
	}`
	p := filepath.Join(t.TempDir(), "two.json")
	if err := os.WriteFile(p, []byte(two), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFixture(p)
	if err != nil {
		t.Fatalf("the fixture did not load: %v", err)
	}
	if len(got.areas) != 2 {
		t.Fatalf("two different places should both stay; got %d", len(got.areas))
	}
}
