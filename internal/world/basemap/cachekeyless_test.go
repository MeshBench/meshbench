package basemap

import (
	"strings"
	"testing"
)

// A tile fetched without a key is not cached where a keyed build will find it.
//
// CARTO serves a tile either way: with a key it is the map, without one it is
// the map under an API KEY REQUIRED watermark. The cache was keyed on style,
// zoom and position and recorded neither, so one run of a locally built binary
// - which has no key, because the key lives in the release pipeline - wrote
// watermarked tiles that every release build installed afterwards re-served for
// ever. Reported from a machine whose key was working perfectly.
func TestAKeylessTileIsCachedApartFromAKeyedOne(t *testing.T) {
	carto := Layer{ID: "carto-dark",
		URL: "https://a.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}.png"}

	t.Setenv("MESHBENCH_CARTO_KEY", "")
	keyless := cacheDirFor(carto)

	t.Setenv("MESHBENCH_CARTO_KEY", "a-real-key")
	keyed := cacheDirFor(carto)

	if keyless == keyed {
		t.Fatalf("both fetches cache under %q, so a keyless run poisons a "+
			"keyed one for ever", keyed)
	}
	if keyed != carto.ID {
		t.Errorf("a keyed build caches under %q, not the layer's own id %q - "+
			"which would strand every tile already fetched correctly",
			keyed, carto.ID)
	}
	if !strings.Contains(keyless, carto.ID) {
		t.Errorf("the keyless directory %q does not name the layer, so what "+
			"is in it cannot be recognised", keyless)
	}
}

// A layer that takes no key is not split in two.
//
// Splitting it would double the disk for a server that returns the same bytes
// either way.
func TestALayerWithoutAKeyIsCachedOnce(t *testing.T) {
	osm := Layer{ID: "osm", URL: "https://tile.openstreetmap.org/{z}/{x}/{y}.png"}
	t.Setenv("MESHBENCH_CARTO_KEY", "")
	without := cacheDirFor(osm)
	t.Setenv("MESHBENCH_CARTO_KEY", "a-real-key")
	with := cacheDirFor(osm)
	if without != osm.ID || with != osm.ID {
		t.Errorf("OpenStreetMap caches under %q and %q; it takes no key and "+
			"should use its own id either way", without, with)
	}
}
