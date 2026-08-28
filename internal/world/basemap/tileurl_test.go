package basemap

import (
	"strings"
	"testing"
)

// A CARTO tile carries the operator's key when one is in the environment,
// and no other source does - OSM's policy has no key parameter, and leaking
// a key to a third party would be worse than a watermark.
func TestCartoKeyOnCartoTilesOnly(t *testing.T) {
	t.Setenv("MESHBENCH_CARTO_KEY", "k123")
	var carto, osm Layer
	for _, l := range Layers() {
		switch l.ID {
		case "carto-dark":
			carto = l
		case "osm":
			osm = l
		}
	}
	if got := tileURL(carto, 6, 31, 20); !strings.HasSuffix(got, "?key=k123") {
		t.Fatalf("carto tile without the key: %s", got)
	}
	if got := tileURL(osm, 6, 31, 20); strings.Contains(got, "key=") {
		t.Fatalf("the key leaked to a non-CARTO source: %s", got)
	}
}

func TestNoKeyNoParameter(t *testing.T) {
	t.Setenv("MESHBENCH_CARTO_KEY", "")
	for _, l := range Layers() {
		if strings.Contains(tileURL(l, 1, 0, 0), "key=") {
			t.Fatalf("%s grew a key from an empty environment", l.ID)
		}
	}
}
