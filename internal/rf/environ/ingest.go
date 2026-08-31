// Ingesting footprints: GeoJSON in, tiles out.
//
// Here rather than in tools/envgen so the conversion is a tested library
// and the tool stays a flag parser around it.
package environ

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
)

type feature struct {
	Type     string `json:"type"`
	Geometry struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	} `json:"geometry"`
	Properties map[string]any `json:"properties"`
}

// IngestStats says what a conversion did.
type IngestStats struct {
	Buildings int
	Tiles     int
	Skipped   int
}

// ErrNoBuildings is returned when a document parses cleanly but yields no
// buildings at all - a wrapper with an empty "features" array, or a stream
// of lines every one of which failed toBuilding. Silence there reads as "no
// obstructions here", which is the one direction of error this package
// exists to rule out.
var ErrNoBuildings = errors.New("environ: ingest produced no buildings")

// IngestGeoJSON reads building footprints - a FeatureCollection or
// newline-delimited features, which is how Microsoft's Global ML Building
// Footprints ship and how osmium/ogr2ogr export OSM - and writes the tile
// tree the store reads. region picks the material profile for buildings
// whose tags do not say.
func IngestGeoJSON(r io.Reader, outDir, region string) (IngestStats, error) {
	byTile := map[[2]int][]Building{}
	var stats IngestStats
	add := func(ft feature) {
		b, ok := toBuilding(ft, region)
		if !ok {
			stats.Skipped++
			return
		}
		x, y, ok := TileFor(b)
		if !ok {
			stats.Skipped++
			return
		}
		byTile[[2]int{x, y}] = append(byTile[[2]int{x, y}], b)
		stats.Buildings++
	}

	isFC, rest := sniffFeatureCollection(r)
	br := bufio.NewReaderSize(rest, 1<<20)
	if isFC {
		var fc struct {
			Type     string    `json:"type"`
			Features []feature `json:"features"`
		}
		if err := json.NewDecoder(br).Decode(&fc); err != nil {
			return stats, err
		}
		for _, ft := range fc.Features {
			add(ft)
		}
		return writeTiles(outDir, byTile, &stats)
	}
	sc := bufio.NewScanner(br)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) < 2 {
			continue
		}
		var ft feature
		if err := json.Unmarshal(line, &ft); err != nil {
			stats.Skipped++
			continue
		}
		add(ft)
	}
	if err := sc.Err(); err != nil {
		return stats, err
	}
	return writeTiles(outDir, byTile, &stats)
}

func writeTiles(outDir string, byTile map[[2]int][]Building, stats *IngestStats) (IngestStats, error) {
	for k, bs := range byTile {
		if err := WriteTile(outDir, k[0], k[1], bs); err != nil {
			return *stats, err
		}
	}
	stats.Tiles = len(byTile)
	if stats.Buildings == 0 {
		return *stats, ErrNoBuildings
	}
	return *stats, nil
}

// sniffFeatureCollection learns whether the document is a single
// FeatureCollection object or a stream of newline-delimited features, by
// reading only the leading "type" field rather than guessing from how much
// of the document fits in a fixed peek window - a window a short but
// complete FeatureCollection fits inside whole, which read as valid JSON on
// its own and was mistaken for one NDJSON line. The bytes consumed while
// looking are replayed ahead of whatever is left unread, so the caller sees
// the whole document either way.
func sniffFeatureCollection(r io.Reader) (isFC bool, rest io.Reader) {
	var captured bytes.Buffer
	dec := json.NewDecoder(io.TeeReader(r, &captured))
	isFC = leadingType(dec) == "FeatureCollection"
	return isFC, io.MultiReader(bytes.NewReader(captured.Bytes()), r)
}

// leadingType reads an object's tokens up to and including its "type" key,
// decoding only that field's value and skipping every other one, so it never
// commits to the shape of whatever comes after.
func leadingType(dec *json.Decoder) string {
	tok, err := dec.Token()
	if err != nil {
		return ""
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return ""
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return ""
		}
		key, _ := keyTok.(string)
		if key == "type" {
			var typ string
			if err := dec.Decode(&typ); err != nil {
				return ""
			}
			return typ
		}
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return ""
		}
	}
	return ""
}

// toBuilding maps one feature: geometry to lat/lon vertices, properties to
// height and material with provenance. GeoJSON is lon,lat; MeshBench is
// lat,lon - the swap the boundary importer already catches at its own door.
func toBuilding(ft feature, region string) (Building, bool) {
	if ft.Geometry.Type != "Polygon" {
		return Building{}, false
	}
	var rings [][][2]float64
	if err := json.Unmarshal(ft.Geometry.Coordinates, &rings); err != nil || len(rings) == 0 {
		return Building{}, false
	}
	outer := rings[0]
	if len(outer) < 3 {
		return Building{}, false
	}
	fp := make([][2]float64, 0, len(outer))
	for _, v := range outer {
		fp = append(fp, [2]float64{v[1], v[0]})
	}

	b := Building{Footprint: fp}
	props := ft.Properties

	// Height by the plan's precedence: explicit, then levels at three
	// metres each, then a stated default - source and confidence carried.
	if h := numProp(props, "height"); h > 0 {
		b.HeightM, b.HeightSource, b.HeightConfidence = h, "dataset", 0.85
	} else if h := numProp(props, "building:levels"); h > 0 {
		b.HeightM, b.HeightSource, b.HeightConfidence = h*3, "levels", 0.6
	} else {
		b.HeightM, b.HeightSource, b.HeightConfidence = 6, "default", 0.3
	}

	b.Type = strProp(props, "building")
	mat, src, conf := InferMaterial(strProp(props, "building:material"), b.Type, region)
	b.Material, b.MaterialSource, b.MaterialConfidence = mat, src, conf
	return b, true
}

func numProp(m map[string]any, key string) float64 {
	switch v := m[key].(type) {
	case float64:
		return v
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	return 0
}

func strProp(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}
