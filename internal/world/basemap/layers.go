// The layer catalogue: what sources this build knows about, and how to choose
// between them.
//
// Split from the store that fetches them because they are two things: this is
// a description of the world's tile services and their terms, and none of it
// touches the network.
package basemap

import "math"

// Kind separates a base layer from something drawn over one.
type Kind string

const (
	// Base layers are opaque and mutually exclusive.
	Base Kind = "base"
	// Overlay layers are mostly transparent and drawn on top — labels, mainly.
	Overlay Kind = "overlay"
)

// Layer is one tiled raster source.
type Layer struct {
	ID   string
	Name string
	Kind Kind
	// Dark reports a layer whose ground is dark, so text drawn over the map
	// can pick an ink that reads on it rather than one chosen for the theme.
	Dark bool

	// URL template with {z}/{x}/{y}.
	URL string

	// MaxZoom is the deepest zoom the source publishes. Asking beyond it
	// returns errors or blank tiles, so requests are clamped and the map
	// upsamples instead — which looks soft rather than broken.
	MaxZoom int

	// Attribution must appear wherever this layer is shown. Not a courtesy:
	// every one of these sources requires it.
	Attribution string

	// Terms is what a human needs to read before this ships.
	Terms string

	// RequiresReview marks a layer whose terms have not been checked against
	// how this application uses it. Such a layer is available to a developer
	// and must not be a default.
	RequiresReview bool
}

// Layers is what this build knows about.
//
// Deliberately few, and every one a source that publishes a documented tile
// service. Scraping a consumer map product is both a licensing problem and a
// technical one — the URLs change without notice — so nothing here does it.
func Layers() []Layer {
	return []Layer{
		{
			// CARTO's basemaps are built to be a background for data, which is
			// exactly this use, and they are quiet enough that a coverage
			// raster over them stays readable. OSM's standard style is not —
			// it is a map in its own right and fights anything drawn on it.
			ID: "carto-light", Name: "Light", Kind: Base, MaxZoom: 20,
			URL:         "https://basemaps.cartocdn.com/light_all/{z}/{x}/{y}.png",
			Attribution: "(c) OpenStreetMap contributors, (c) CARTO",
			Terms: "CARTO basemaps. Free for use with attribution; a token is " +
				"required above their fair-use volume, which a workbench prefetching " +
				"whole counties could reach.",
			RequiresReview: true,
		},
		{
			ID: "carto-dark", Name: "Dark", Kind: Base, Dark: true, MaxZoom: 20,
			URL:            "https://basemaps.cartocdn.com/dark_all/{z}/{x}/{y}.png",
			Attribution:    "(c) OpenStreetMap contributors, (c) CARTO",
			Terms:          "As CARTO light above.",
			RequiresReview: true,
		},
		{
			ID: "osm", Name: "Roads", Kind: Base, MaxZoom: 19,
			URL:         "https://tile.openstreetmap.org/{z}/{x}/{y}.png",
			Attribution: "(c) OpenStreetMap contributors",
			Terms: "OpenStreetMap Foundation tile usage policy. Requires a valid " +
				"identifying User-Agent, forbids bulk downloading, and expects heavy " +
				"users to run their own tile server. A workbench prefetching a county " +
				"is plausibly bulk downloading.",
			RequiresReview: true,
		},
		{
			ID: "esri-imagery", Name: "Satellite", Kind: Base, Dark: true, MaxZoom: 19,
			URL: "https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/" +
				"MapServer/tile/{z}/{y}/{x}",
			Attribution: "Imagery (c) Esri, Maxar, Earthstar Geographics",
			Terms: "Esri ArcGIS Online basemap. Free tiers exist for some uses and " +
				"not others; caching to disk in particular is restricted under some " +
				"Esri terms.",
			RequiresReview: true,
		},
		{
			ID: "esri-topo", Name: "Topographic", Kind: Base, MaxZoom: 19,
			URL: "https://server.arcgisonline.com/ArcGIS/rest/services/World_Topo_Map/" +
				"MapServer/tile/{z}/{y}/{x}",
			Attribution:    "(c) Esri, contributors",
			Terms:          "As Esri imagery above.",
			RequiresReview: true,
		},
		{
			// CARTO publishes its labels as their own transparent layer,
			// which is what lets place names ride ABOVE a coverage raster
			// instead of drowning under it.
			ID: "carto-dark-labels", Name: "Labels (dark)", Kind: Overlay, Dark: true, MaxZoom: 20,
			URL:            "https://basemaps.cartocdn.com/dark_only_labels/{z}/{x}/{y}.png",
			Attribution:    "(c) OpenStreetMap contributors, (c) CARTO",
			Terms:          "As CARTO light above.",
			RequiresReview: true,
		},
		{
			ID: "carto-light-labels", Name: "Labels (light)", Kind: Overlay, MaxZoom: 20,
			URL:            "https://basemaps.cartocdn.com/light_only_labels/{z}/{x}/{y}.png",
			Attribution:    "(c) OpenStreetMap contributors, (c) CARTO",
			Terms:          "As CARTO light above.",
			RequiresReview: true,
		},
		{
			// MaxZoom is what the service actually draws, not what it
			// answers: past 17 it returns 200s full of transparent
			// nothing, which cached as roads that vanish when zoomed in.
			ID: "esri-roads", Name: "Roads (overlay)", Kind: Overlay, MaxZoom: 17,
			URL: "https://server.arcgisonline.com/ArcGIS/rest/services/Reference/" +
				"World_Transportation/MapServer/tile/{z}/{y}/{x}",
			Attribution:    "(c) Esri, contributors",
			Terms:          "As Esri imagery above.",
			RequiresReview: true,
		},
		{
			ID: "esri-labels", Name: "Places and roads", Kind: Overlay, MaxZoom: 17,
			URL: "https://server.arcgisonline.com/ArcGIS/rest/services/Reference/" +
				"World_Boundaries_and_Places/MapServer/tile/{z}/{y}/{x}",
			Attribution:    "(c) Esri, contributors",
			Terms:          "As Esri imagery above.",
			RequiresReview: true,
		},
	}
}

// OverlaysFor is what belongs above a coverage raster for a given base:
// roads first, then labels, so the ground's structure and its names both
// come through the picture instead of drowning under it. Bases with
// everything baked in and no separate layers answer empty.
func OverlaysFor(baseID string) []Layer {
	// The streets come through the raster as a ghost of the base itself -
	// every road at every zoom, no second source, no doubled names - so
	// the overlay stack is labels only, from the base's own family. The
	// exceptions carry Esri layers because their base has nothing baked
	// to ghost (imagery) or no separate labels of its own.
	ids := map[string][]string{
		"carto-dark":   {"carto-dark-labels"},
		"carto-light":  {"carto-light-labels"},
		"esri-imagery": {"esri-roads", "esri-labels"},
		"esri-topo":    {"esri-labels"},
	}[baseID]
	var out []Layer
	for _, id := range ids {
		if l, ok := ByID(id); ok {
			out = append(out, l)
		}
	}
	return out
}

// ByID finds a layer.
func ByID(id string) (Layer, bool) {
	for _, l := range Layers() {
		if l.ID == id {
			return l, true
		}
	}
	return Layer{}, false
}

// ZoomFor picks the tile zoom whose pixels are nearest the view's own scale.
//
// Capped at the layer's maximum, and never below 1. ADR-0019's finding applies
// unchanged: naive zoom selection is what turns a map pan into a 700-tile
// download, and choosing the zoom that matches the screen is what stops it.
func ZoomFor(metresPerPixel, latDeg float64, l Layer) int {
	if metresPerPixel <= 0 {
		return 12
	}
	// Web Mercator ground resolution at zoom z.
	const equator = 156543.033928
	res := equator * math.Cos(latDeg*math.Pi/180)
	z := int(math.Round(math.Log2(res / metresPerPixel)))
	maxZoom := l.MaxZoom
	if maxZoom <= 0 {
		maxZoom = 18
	}
	if z > maxZoom {
		z = maxZoom
	}
	if z < 1 {
		z = 1
	}
	return z
}
