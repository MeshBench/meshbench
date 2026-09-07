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
	for _, d := range []string{keyed, keyless} {
		if !strings.Contains(d, carto.ID) {
			t.Errorf("the directory %q does not name the layer, so what is in "+
				"it cannot be recognised", d)
		}
	}
}

// Neither side keeps the layer's bare id, because that is where the mixed
// tiles already are.
//
// The first fix left the keyed build on l.ID so tiles already fetched
// correctly were not stranded. But every tile written before the split is in
// that directory, keyed and keyless together and nothing to tell them apart -
// so a keyed build reading it still serves whatever a keyless run left there,
// which is the machine this was reported from. Retiring the id costs a refetch
// of the tiles somebody actually looks at; keeping it costs the fix.
func TestNeitherSideReadsWhatWasCachedBeforeTheSplit(t *testing.T) {
	carto := Layer{ID: "carto-dark",
		URL: "https://a.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}.png"}

	t.Setenv("MESHBENCH_CARTO_KEY", "a-real-key")
	if got := cacheDirFor(carto); got == carto.ID {
		t.Errorf("a keyed build still reads %q, where the pre-split tiles are, "+
			"so a poisoned cache stays poisoned", got)
	}
	t.Setenv("MESHBENCH_CARTO_KEY", "")
	if got := cacheDirFor(carto); got == carto.ID {
		t.Errorf("a keyless build still writes to %q, where a keyed build "+
			"used to read", got)
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
