package scenario

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseGeoJSON turns a GeoJSON document into boundaries.
//
// Accepts a FeatureCollection, a single Feature, or a bare geometry, because
// every source hands out a different one of the three and a user pasting a file
// should not have to know which they were given.
//
// Only Polygon and MultiPolygon carry area. Points and lines are skipped rather
// than rejected: national datasets routinely mix a boundary polygon with a
// label point, and refusing the whole file over the label helps nobody.
func ParseGeoJSON(data []byte, nameField string) ([]Boundary, error) {
	var doc struct {
		Type     string            `json:"type"`
		Features []json.RawMessage `json:"features"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("scenario: not GeoJSON: %w", err)
	}

	switch strings.ToLower(doc.Type) {
	case "featurecollection":
		var out []Boundary
		for i, raw := range doc.Features {
			bs, err := parseFeature(raw, nameField)
			if err != nil {
				return nil, fmt.Errorf("scenario: feature %d: %w", i, err)
			}
			out = append(out, bs...)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("scenario: GeoJSON has no polygons; %d features, all points or lines", len(doc.Features))
		}
		return out, nil
	case "feature":
		return parseFeature(data, nameField)
	case "polygon", "multipolygon":
		return parseGeometry(data, "")
	default:
		return nil, fmt.Errorf("scenario: unsupported GeoJSON type %q", doc.Type)
	}
}

func parseFeature(raw []byte, nameField string) ([]Boundary, error) {
	var f struct {
		Properties map[string]any  `json:"properties"`
		Geometry   json.RawMessage `json:"geometry"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	if len(f.Geometry) == 0 {
		return nil, nil
	}
	return parseGeometry(f.Geometry, featureName(f.Properties, nameField))
}

// featureName picks a human label.
//
// Tries the caller's field first, then the names these datasets actually use.
// A boundary with no label is still usable but much harder to talk about, and
// "Scotland" versus "feature 3" is the difference in every error message the
// user will ever see from it.
func featureName(props map[string]any, field string) string {
	candidates := []string{field, "name", "NAME", "ctry19nm", "lad19nm", "NAME_1", "admin", "title"}
	for _, k := range candidates {
		if k == "" {
			continue
		}
		if v, ok := props[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func parseGeometry(raw []byte, name string) ([]Boundary, error) {
	var g struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		return nil, err
	}

	switch strings.ToLower(g.Type) {
	case "polygon":
		var rings [][][2]float64
		if err := json.Unmarshal(g.Coordinates, &rings); err != nil {
			return nil, fmt.Errorf("polygon coordinates: %w", err)
		}
		b, ok := boundaryFromRings(name, rings)
		if !ok {
			return nil, nil
		}
		return []Boundary{b}, nil

	case "multipolygon":
		var polys [][][][2]float64
		if err := json.Unmarshal(g.Coordinates, &polys); err != nil {
			return nil, fmt.Errorf("multipolygon coordinates: %w", err)
		}
		var out []Boundary
		for _, rings := range polys {
			// Each part keeps the parent's name. Scotland is one boundary made
			// of the mainland and several hundred islands, and a node on Islay
			// should report as being in Scotland, not in "Scotland part 118".
			b, ok := boundaryFromRings(name, rings)
			if !ok {
				continue
			}
			out = append(out, b)
		}
		return out, nil

	case "point", "multipoint", "linestring", "multilinestring":
		return nil, nil // no area; not an error

	case "geometrycollection":
		var gc struct {
			Geometries []json.RawMessage `json:"geometries"`
		}
		if err := json.Unmarshal(raw, &gc); err != nil {
			return nil, err
		}
		var out []Boundary
		for _, sub := range gc.Geometries {
			bs, err := parseGeometry(sub, name)
			if err != nil {
				return nil, err
			}
			out = append(out, bs...)
		}
		return out, nil

	default:
		return nil, fmt.Errorf("unsupported geometry %q", g.Type)
	}
}

// boundaryFromRings converts GeoJSON rings, which are [lon, lat] — the opposite
// order to how everything else in this codebase writes a coordinate, and the
// single most common way to end up with a boundary in the Indian Ocean.
func boundaryFromRings(name string, rings [][][2]float64) (Boundary, bool) {
	if len(rings) == 0 || len(rings[0]) < 3 {
		return Boundary{}, false
	}
	b := Boundary{Name: name}
	for i, ring := range rings {
		r := make(Ring, 0, len(ring))
		for _, c := range ring {
			r = append(r, LatLon{Lat: c[1], Lon: c[0]})
		}
		if i == 0 {
			b.Rings = []Ring{r}
		} else {
			// Interior rings are holes: lochs, enclaves, and the hole in the
			// middle of a district that contains a city. Treating them as extra
			// outers would include exactly the area meant to be excluded.
			b.Holes = append(b.Holes, r)
		}
	}
	return b, true
}
