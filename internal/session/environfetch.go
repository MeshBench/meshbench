// Downloading building footprints at runtime.
//
// tools/envgen remains the way to prepare a region properly; this is the
// impatient path - pick a database in Configuration, pull what covers the
// loaded network, and test buildings without leaving the application. Only
// data crosses the network, the result is cached permanently like terrain,
// and a pull that would be enormous fails loudly rather than trying.
package session

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/environ"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// environMarginKm is how far past the outermost node footprints are pulled.
// Buildings only matter where paths run, and paths run between nodes.
const environMarginKm = 5

// overpassMaxKm2 caps a live Overpass pull. Beyond this the polite answer is
// tools/envgen over a regional extract, and the error says so.
const overpassMaxKm2 = 4000

// microsoftMaxFiles caps how many quadkey files one pull will download.
const microsoftMaxFiles = 16

var environClient = &http.Client{Timeout: 15 * time.Minute}

// environBox is the pull's footprint: the network's bounding box plus margin.
func environBox(nodes []scenario.Node) (south, north, west, east float64, err error) {
	if len(nodes) == 0 {
		return 0, 0, 0, 0, fmt.Errorf("no nodes loaded; a pull needs a map to cover")
	}
	south, north = math.Inf(1), math.Inf(-1)
	west, east = math.Inf(1), math.Inf(-1)
	for _, n := range nodes {
		south = math.Min(south, n.Position.Lat)
		north = math.Max(north, n.Position.Lat)
		west = math.Min(west, n.Position.Lon)
		east = math.Max(east, n.Position.Lon)
	}
	midLat := (south + north) / 2
	dLat := environMarginKm / 111.32
	dLon := environMarginKm / (111.32 * math.Cos(midLat*math.Pi/180))
	return south - dLat, north + dLat, west - dLon, east + dLon, nil
}

func boxAreaKm2(south, north, west, east float64) float64 {
	midLat := (south + north) / 2
	return (north - south) * 111.32 * (east - west) * 111.32 * math.Cos(midLat*math.Pi/180)
}

// environCacheDir is where one pull's tiles live: keyed by source and box, so
// moving the network pulls fresh ground and reopening the same network reuses
// the cache without asking.
func environCacheDir(source string, south, north, west, east float64) (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(fmt.Appendf(nil, "%.4f/%.4f/%.4f/%.4f", south, north, west, east))
	return filepath.Join(cache, "meshcoresim", "environment",
		fmt.Sprintf("%s-%x", source, sum[:4])), nil
}

// hasTiles reports whether a previous pull already landed here. The store
// lays tiles out as z<zoom>/x/y.jsonl.gz, so the zoom directory existing
// with anything in it is the fact that matters.
func hasTiles(dir string) bool {
	entries, err := os.ReadDir(filepath.Join(dir, fmt.Sprintf("z%d", environ.TileZoom)))
	return err == nil && len(entries) > 0
}

// overpassNDJSON pulls OSM building ways in the box and rewrites them as the
// newline-delimited GeoJSON the ingester reads. Relations (multipolygons) are
// left to envgen: their outer rings need assembly this path does not attempt,
// and silently mangling them would be worse than saying so.
func overpassNDJSON(south, north, west, east float64) (io.Reader, int, error) {
	q := fmt.Sprintf(`[out:json][timeout:180];way["building"](%f,%f,%f,%f);out geom;`,
		south, west, north, east)
	resp, err := environClient.PostForm("https://overpass-api.de/api/interpreter",
		map[string][]string{"data": {q}})
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("overpass answered %s", resp.Status)
	}
	return overpassToNDJSON(resp.Body)
}

// overpassToNDJSON rewrites an Overpass answer as the newline-delimited
// GeoJSON the ingester reads. Split from the request so the rewrite is
// testable without a network.
func overpassToNDJSON(body io.Reader) (io.Reader, int, error) {
	var parsed struct {
		Elements []struct {
			Type     string `json:"type"`
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

// fetchEnviron is the pull, run off the store's goroutine: resolve, download,
// ingest into the cache, then hand the directory to rf.environment exactly as
// if the operator had typed it.
func (s *Sim) fetchEnviron(source string, south, north, west, east float64,
	progress func(done, total int)) (string, environ.IngestStats, error) {
	dir, err := environCacheDir(source, south, north, west, east)
	if err != nil {
		return "", environ.IngestStats{}, err
	}
	if hasTiles(dir) {
		return dir, environ.IngestStats{}, nil
	}
	var rd io.Reader
	var done func()
	switch source {
	case "osm":
		if a := boxAreaKm2(south, north, west, east); a > overpassMaxKm2 {
			return "", environ.IngestStats{}, fmt.Errorf(
				"this map covers %.0f km2, past the %.0f km2 a live Overpass "+
					"pull is fair for; prepare the region offline with tools/envgen",
				a, float64(overpassMaxKm2))
		}
		var n int
		rd, n, err = overpassNDJSON(south, north, west, east)
		if err == nil && n == 0 {
			err = fmt.Errorf("OpenStreetMap has no building ways in this map's area")
		}
		progress(1, 2)
	case "microsoft":
		urls, uerr := microsoftURLs(south, north, west, east)
		if uerr != nil {
			return "", environ.IngestStats{}, uerr
		}
		rd, done, err = microsoftNDJSON(urls, func(d int) { progress(d, len(urls)+1) })
	default:
		return "", environ.IngestStats{}, fmt.Errorf("no building database %q; there is osm and microsoft", source)
	}
	if err != nil {
		return "", environ.IngestStats{}, err
	}
	if done != nil {
		defer done()
	}
	stats, err := environ.IngestGeoJSON(rd, dir, "uk")
	if err != nil {
		return "", stats, err
	}
	return dir, stats, nil
}

func registerEnvironFetch(st *state.Store, s *Sim) {
	// environ.fetch: pull a building database for the loaded map, cache it,
	// and switch it on. The heavy part runs as a job; what lands back on the
	// store's goroutine is only the outcome.
	st.Handle("environ.fetch", func(w *state.World, p any) (any, error) {
		source, _ := stringField(p, "source")
		if source == "" {
			source = "osm"
		}
		south, north, west, east, err := environBox(s.nodes)
		if err != nil {
			return nil, err
		}
		const id = "environ-fetch"
		what := "buildings: " + source
		w.Jobs = append(w.Jobs, state.Job{ID: id, What: what, Total: 1})
		w.Say(fmt.Sprintf("pulling %s footprints for the map's area", source))
		go func() {
			ctx := context.Background()
			dir, stats, err := s.fetchEnviron(source, south, north, west, east,
				func(done, total int) {
					_, _ = st.Do(ctx, "job.progress", state.Job{
						ID: id, What: what, Done: done, Total: total})
				})
			if err != nil {
				_, _ = st.Do(ctx, "environ.failed", err.Error())
				return
			}
			note := "already cached"
			if stats.Buildings > 0 {
				note = fmt.Sprintf("%d building(s) into %d tile(s), %d skipped",
					stats.Buildings, stats.Tiles, stats.Skipped)
			}
			_, _ = st.Do(ctx, "environ.fetched", note)
			// The same verb the manual path uses, so there is exactly one
			// way buildings get switched on.
			_, _ = st.Do(ctx, "rf.environment", map[string]any{"dir": dir})
		}()
		return map[string]any{"source": source, "started": true}, nil
	})

	st.Handle("environ.fetched", func(w *state.World, p any) (any, error) {
		w.Jobs = finishJob(w.Jobs, "environ-fetch")
		w.Say("footprints ready: " + soleString(p))
		return nil, nil
	})

	st.Handle("environ.failed", func(w *state.World, p any) (any, error) {
		w.Jobs = finishJob(w.Jobs, "environ-fetch")
		w.Say("building pull failed: " + soleString(p))
		return nil, nil
	})
}
