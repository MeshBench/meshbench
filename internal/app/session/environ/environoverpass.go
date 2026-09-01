// The OpenStreetMap half of a runtime pull, over the Overpass API.
//
// Kept beside the Microsoft half rather than in with the verb, because the
// two sources are the thing most likely to change independently: a server
// that starts refusing, a query that has to be rewritten, a cap that turns
// out to be the wrong number.
package environ

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
)

// overpassMaxKm2 caps a live Overpass pull, summed over the patches. Beyond
// this the polite answer is tools/envgen over a regional extract, and the
// error says so. The pull goes to the server one small chunk at a time, so
// the cap is about total volume, not any single request.
const overpassMaxKm2 = 8000

// overpassChunk is how many patches one Overpass request carries.
const overpassChunk = 10

// overpassNDJSON pulls OSM building ways in every patch, a chunk of patches
// per request so a national network is many small queries with progress
// rather than one enormous one. A way straddling two chunks would arrive
// twice - a building priced twice is a wall paid twice - so ways are
// deduplicated by id across the whole pull. Relations (multipolygons) are
// left to envgen: their outer rings need assembly this path does not
// attempt, and silently mangling them would be worse than saying so.
func overpassNDJSON(ctx context.Context, patches []llBox, progress func(done, total int)) (io.Reader, int, error) {
	chunks := (len(patches) + overpassChunk - 1) / overpassChunk
	seen := map[int64]bool{}
	var all strings.Builder
	total := 0
	for c := 0; c < chunks; c++ {
		hi := (c + 1) * overpassChunk
		if hi > len(patches) {
			hi = len(patches)
		}
		var union strings.Builder
		for _, b := range patches[c*overpassChunk : hi] {
			fmt.Fprintf(&union, `way["building"](%f,%f,%f,%f);`, b.South, b.West, b.North, b.East)
		}
		q := fmt.Sprintf(`[out:json][timeout:180];(%s);out geom;`, union.String())
		resp, err := environDo(ctx, "POST", "https://overpass-api.de/api/interpreter",
			"application/x-www-form-urlencoded", "data="+neturl.QueryEscape(q))
		if err != nil {
			return nil, 0, err
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, 0, fmt.Errorf("overpass answered %s", resp.Status)
		}
		part, n, err := overpassToNDJSON(resp.Body, seen)
		_ = resp.Body.Close()
		if err != nil {
			return nil, 0, err
		}
		if _, err := io.Copy(&all, part); err != nil {
			return nil, 0, err
		}
		total += n
		if progress != nil {
			progress(c+1, chunks)
		}
	}
	return strings.NewReader(all.String()), total, nil
}

// overpassToNDJSON rewrites an Overpass answer as the newline-delimited
// GeoJSON the ingester reads. Split from the request so the rewrite is
// testable without a network.
func overpassToNDJSON(body io.Reader, seen map[int64]bool) (io.Reader, int, error) {
	var parsed struct {
		Elements []struct {
			Type     string `json:"type"`
			ID       int64  `json:"id"`
			Geometry []struct {
				Lat float64 `json:"lat"`
				Lon float64 `json:"lon"`
			} `json:"geometry"`
			Tags map[string]string `json:"tags"`
		} `json:"elements"`
	}
	if err := json.NewDecoder(body).Decode(&parsed); err != nil {
		return nil, 0, err
	}
	var out strings.Builder
	n := 0
	for _, el := range parsed.Elements {
		if el.Type != "way" || len(el.Geometry) < 3 {
			continue
		}
		if seen != nil {
			if seen[el.ID] {
				continue
			}
			seen[el.ID] = true
		}
		ring := make([][2]float64, 0, len(el.Geometry))
		for _, pt := range el.Geometry {
			ring = append(ring, [2]float64{pt.Lon, pt.Lat})
		}
		props := map[string]any{}
		for k, v := range el.Tags {
			props[k] = v
		}
		line, err := json.Marshal(map[string]any{
			"type": "Feature",
			"geometry": map[string]any{
				"type": "Polygon", "coordinates": [][][2]float64{ring},
			},
			"properties": props,
		})
		if err != nil {
			continue
		}
		out.Write(line)
		out.WriteByte('\n')
		n++
	}
	return strings.NewReader(out.String()), n, nil
}
