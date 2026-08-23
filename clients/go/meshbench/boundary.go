// The study area: which nodes are in the question being asked.
//
// Not the firmware's region concept. A boundary decides what is studied; a
// region decides what is forwarded. Both words are in this application, and
// confusing them is how somebody concludes the RF model is broken.
//
// Set it before importing. The import filters at fetch time, so a boundary set
// afterwards prunes what has already been paid for rather than never fetching
// it.
package meshbench

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Boundary is the study area, however you have it. Live.
type Boundary struct{ w *Workbench }

// Boundary reaches the study area.
func (w *Workbench) Boundary() Boundary { return Boundary{w} }

// Use takes a study area from a place name or from GeoJSON.
//
// The one to call. A path to a .geojson file is loaded; anything else is
// searched for by name and the best match accepted. Both end with the area in
// the study, which is the only thing the caller wanted to say.
func (b Boundary) Use(ctx context.Context, area string) ([]string, error) {
	if isGeoJSONPath(area) {
		return b.Load(ctx, area, "")
	}
	found, err := b.Search(ctx, area)
	if err != nil {
		return nil, err
	}
	name, err := b.Accept(ctx, found[0])
	if err != nil {
		return nil, err
	}
	return []string{name}, nil
}

// Search finds places matching a name, best first. Needs the network.
//
// Names rather than geometry: the geometry stays at the workbench, and the
// name is what Accept takes.
func (b Boundary) Search(ctx context.Context, query string) ([]string, error) {
	var out struct {
		Names []string `json:"names"`
	}
	if err := b.w.CallInto(ctx, "boundary.set",
		map[string]any{"query": query}, &out); err != nil {
		return nil, err
	}
	if len(out.Names) == 0 {
		return nil, &Refused{Verb: "boundary.set", Code: "not_found",
			Message: fmt.Sprintf("nothing is called %q", query), kind: ErrNotFound}
	}
	return out.Names, nil
}

// Accept takes one of the search results into the study area.
//
// Areas union rather than replace: a study is often two council areas rather
// than one.
func (b Boundary) Accept(ctx context.Context, name string) (string, error) {
	var out struct {
		Accepted string `json:"accepted"`
	}
	err := b.w.CallInto(ctx, "boundary.accept", map[string]any{"name": name}, &out)
	return out.Accepted, err
}

// Load takes a study area from GeoJSON: a path, or the document itself.
//
// A Polygon, a MultiPolygon, a Feature or a FeatureCollection. Each polygon
// becomes an area named from its "name" property, or from name, or from the
// file.
//
// The one way to study an area nothing has an administrative name for - a
// catchment, a valley, the bit north of the river - and the only one that
// works with no network at all.
func (b Boundary) Load(ctx context.Context, source, name string) ([]string, error) {
	params := map[string]any{}
	switch {
	case isGeoJSONPath(source):
		params["path"] = source
	case json.Valid([]byte(source)):
		params["geojson"] = source
	default:
		// Said here rather than at the workbench, which would report it as a
		// parse failure on a document that is really a mistyped path.
		return nil, fmt.Errorf(
			"meshbench: %q is neither a .geojson path that exists nor a GeoJSON document",
			source)
	}
	if name != "" {
		params["name"] = name
	}
	var out struct {
		Loaded []string `json:"loaded"`
	}
	return out.Loaded, b.w.CallInto(ctx, "boundary.load", params, &out)
}

// List is what the study area is made of.
func (b Boundary) List(ctx context.Context) ([]string, error) {
	var out struct {
		Names []string `json:"names"`
	}
	return out.Names, b.w.CallInto(ctx, "boundary.list", nil, &out)
}

// Remove takes one area back out.
//
// Changes what is measured, never what is loaded: the nodes stay until
// something prunes them.
func (b Boundary) Remove(ctx context.Context, name string) error {
	return b.w.Do(ctx, "boundary.remove", map[string]any{"name": name})
}

// Prune deletes the nodes outside the study area, and says how many went.
//
// For a mesh that was imported before the boundary was set. marginKm is kept
// on purpose and zero means the session's own: a node just outside still
// interferes with one just inside, and dropping it makes the inside look
// quieter than it is.
func (b Boundary) Prune(ctx context.Context, marginKm float64) (int, error) {
	params := map[string]any{}
	if marginKm > 0 {
		params["margin_km"] = marginKm
	}
	var out struct {
		Removed int `json:"removed"`
	}
	return out.Removed, b.w.CallInto(ctx, "boundary.prune", params, &out)
}

// isGeoJSONPath tells a file from a place name or a document.
//
// By extension as well as by existence, so a mistyped path is reported as a
// missing file rather than searched for as a place - which answers "nothing is
// called ./bounds/fife.geojson" and sends the reader in entirely the wrong
// direction.
func isGeoJSONPath(s string) bool {
	if strings.HasPrefix(strings.TrimSpace(s), "{") {
		return false
	}
	if strings.HasSuffix(s, ".geojson") || strings.HasSuffix(s, ".json") {
		return true
	}
	st, err := os.Stat(s)
	return err == nil && !st.IsDir()
}
