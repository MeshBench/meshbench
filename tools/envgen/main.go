// envgen turns building footprints into MeshBench environment tiles.
//
// Input is GeoJSON - a FeatureCollection or newline-delimited features,
// which is how Microsoft's Global ML Building Footprints ship and how
// osmium/ogr2ogr export OSM extracts. Output is the tile tree
// internal/environ reads. Offline by design: fetching the datasets is the
// operator's step (they are per-region and enormous); turning them into
// tiles is this one.
//
//	envgen -in fife.geojsonl -out ~/.cache/meshcoresim/environment -region uk
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/MeshBench/meshbench/internal/rf/environ"
)

func main() {
	in := flag.String("in", "", "GeoJSON or GeoJSONL of building footprints")
	out := flag.String("out", "", "tile directory (internal/environ format)")
	region := flag.String("region", "uk", "regional material profile")
	flag.Parse()
	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "envgen -in buildings.geojsonl -out DIR [-region uk]")
		os.Exit(2)
	}
	f, err := os.Open(*in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "envgen:", err)
		os.Exit(1)
	}
	defer func() { _ = f.Close() }()
	stats, err := environ.IngestGeoJSON(f, *out, *region)
	if err != nil {
		fmt.Fprintln(os.Stderr, "envgen:", err)
		os.Exit(1)
	}
	fmt.Printf("envgen: %d buildings into %d tiles (%d features skipped)\n",
		stats.Buildings, stats.Tiles, stats.Skipped)
}
