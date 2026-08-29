package basemap

import (
	"os"
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

// The environment always beats the build-stamped default, and the stamp
// alone is enough for a keyed tile.
func TestEnvironmentBeatsTheBakedDefault(t *testing.T) {
	old := defaultCartoKey
	defaultCartoKey = "baked"
	defer func() { defaultCartoKey = old }()

	t.Setenv("MESHBENCH_CARTO_KEY", "")
	var carto Layer
	for _, l := range Layers() {
		if l.ID == "carto-dark" {
			carto = l
		}
	}
	if got := tileURL(carto, 6, 31, 20); !strings.HasSuffix(got, "?key=baked") {
		t.Fatalf("the baked default did not reach the tile: %s", got)
	}
	t.Setenv("MESHBENCH_CARTO_KEY", "mine")
	if got := tileURL(carto, 6, 31, 20); !strings.HasSuffix(got, "?key=mine") {
		t.Fatalf("the environment did not win: %s", got)
	}
}

// A .carto-key file in the working directory carries the key for a source
// checkout: above the baked default, below the environment.
func TestCartoKeyFileBeatsBakedAndLosesToEnv(t *testing.T) {
	old := defaultCartoKey
	defaultCartoKey = "baked"
	defer func() { defaultCartoKey = old }()

	dir := t.TempDir()
	if err := os.WriteFile(dir+"/.carto-key", []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	t.Setenv("MESHBENCH_CARTO_KEY", "")
	if got := CartoKey(); got != "from-file" {
		t.Fatalf("file did not win over the baked default: %q", got)
	}
	t.Setenv("MESHBENCH_CARTO_KEY", "mine")
	if got := CartoKey(); got != "mine" {
		t.Fatalf("environment did not win over the file: %q", got)
	}
}
