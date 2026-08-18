// Custom study areas, drawn or exported rather than searched.
//
// Nominatim answers a country, a council, a national park - anything with a
// published administrative boundary. Most regions provisioning needs to
// target are smaller than that: a repeater group's own patch, three glens a
// study is about. Nothing publishes those, so they have to be imported from a
// file somebody drew - geojson.io, QGIS, a council's open-data portal.
//
// scenario.ParseGeoJSON already does the parsing, and Nominatim's own
// responses already go through it - so an imported file and a searched
// boundary are the same kind of object by the time anything else uses them.
// What was missing was a way in: RegionFromGeoJSONFile existed, called from
// nowhere.
package session

import (
	"fmt"
	"os"

	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/scenario"
)

func registerBoundaryGeoJSON(st *state.Store, s *Sim) {
	// boundary.import: a GeoJSON file becomes a study area, exactly like an
	// accepted search result - unioned into the same list, usable by every
	// condition, prune and coverage tool that already reads it.
	st.Handle("boundary.import", func(w *state.World, p any) (any, error) {
		path, _ := stringField(p, "path")
		if path == "" {
			return nil, fmt.Errorf("boundary.import needs a path to a GeoJSON file")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		name, _ := stringField(p, "name")
		bounds, err := scenario.ParseGeoJSON(data, "")
		if err != nil {
			return nil, fmt.Errorf("%s does not look like GeoJSON: %w", path, err)
		}
		if len(bounds) == 0 {
			return nil, fmt.Errorf("%s has no polygons in it", path)
		}
		if name == "" {
			name = bounds[0].Name
		}
		if name == "" {
			name = baseNameWithoutExt(path)
		}
		for i := range bounds {
			bounds[i].Name, bounds[i].Source = name, "geojson"
		}

		area := state.Area{Name: name, Source: "geojson"}
		for _, b := range bounds {
			for _, r := range b.Rings {
				area.Rings = append(area.Rings, ringToPoints(r))
			}
			for _, h := range b.Holes {
				area.Holes = append(area.Holes, ringToPoints(h))
			}
		}
		w.Areas = append(w.Areas, area)
		s.areas = append(s.areas, bounds...)

		inside := 0
		region := scenario.Region{Boundaries: bounds}
		for _, n := range s.nodes {
			if region.Contains(n.Position) {
				inside++
			}
		}
		// GeoJSON is longitude-then-latitude; a file that went through a tool
		// expecting the other order parses without error and describes a
		// polygon nowhere near this network. Zero matches on a network that
		// is not empty is the visible symptom, said here rather than left for
		// a rule to discover by silently matching nobody.
		note := ""
		if inside == 0 && len(s.nodes) > 0 {
			note = "no nodes fall inside this area - if it looks right on a map, " +
				"check the file was written lon,lat rather than lat,lon"
		}
		w.Say(fmt.Sprintf("imported %q from %s; %d nodes inside it", name, path, inside))
		return map[string]any{
			"name": name, "areas": len(w.Areas), "nodes_inside": inside, "note": note,
		}, nil
	})
}

func ringToPoints(r scenario.Ring) []state.Point {
	out := make([]state.Point, len(r))
	for i, pt := range r {
		out[i] = state.Point{Lat: pt.Lat, Lon: pt.Lon}
	}
	return out
}

func baseNameWithoutExt(path string) string {
	base := path
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			base = path[i+1:]
			break
		}
	}
	for i := len(base) - 1; i >= 0; i-- {
		if base[i] == '.' {
			return base[:i]
		}
	}
	return base
}
