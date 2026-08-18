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
	neturl "net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/environ"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// environPatchKm is how far around each node footprints are pulled. A
// building changes a path where the path is low and close - at its ends -
// so the pull is node-centred patches, not the network's bounding box: a
// national network's box is mostly sea and empty country, and pulling it
// would be refused for size while every town that matters went unserved.
const environPatchKm = 2.5

// overpassMaxKm2 caps a live Overpass pull, summed over the patches. Beyond
// this the polite answer is tools/envgen over a regional extract, and the
// error says so. The pull goes to the server one small chunk at a time, so
// the cap is about total volume, not any single request.
const overpassMaxKm2 = 8000

// overpassChunk is how many patches one Overpass request carries.
const overpassChunk = 10

// microsoftMaxBytes caps a Microsoft pull by what the index says the files
// weigh. Disk is bounded separately - one file lives at a time - so this is
// about bandwidth and patience, and the failure message prices both.
const microsoftMaxBytes = 8e9

var environClient = &http.Client{Timeout: 15 * time.Minute}

// environUserAgent identifies these requests. Not a courtesy: Overpass's
// operators block Go's default agent outright - 406 Not Acceptable in a
// fifth of a second - which is exactly how every building pull failed
// while looking like a download that never started.
const environUserAgent = "MeshBench/1.0 (RF mesh simulator; building-footprint fetch)"

// environGet is Get with the identity, and a polite retry on the
// throttling answers.
func environGet(url string) (*http.Response, error) {
	return environDo("GET", url, "", "")
}

func environDo(method, url, contentType, body string) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		var rd io.Reader
		if body != "" {
			rd = strings.NewReader(body)
		}
		req, err := http.NewRequest(method, url, rd)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", environUserAgent)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		resp, err := environClient.Do(req)
		if err != nil {
			return nil, err
		}
		// 429 and 504 are Overpass saying "not right now": wait and ask
		// again, twice, before giving up loudly.
		if (resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode == http.StatusGatewayTimeout) && attempt < 2 {
			_ = resp.Body.Close()
			time.Sleep(time.Duration(20*(attempt+1)) * time.Second)
			continue
		}
		return resp, nil
	}
}

// llBox is one patch of ground, in degrees.
type llBox struct{ South, North, West, East float64 }

// environPatches is the pull's footprint: a patch around every node, with
// overlapping patches merged so a town of nodes is asked for once.
func environPatches(nodes []scenario.Node) ([]llBox, error) {
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes loaded; a pull needs a map to cover")
	}
	boxes := make([]llBox, 0, len(nodes))
	for _, n := range nodes {
		dLat := environPatchKm / 111.32
		dLon := environPatchKm / (111.32 * math.Cos(n.Position.Lat*math.Pi/180))
		boxes = append(boxes, llBox{
			South: n.Position.Lat - dLat, North: n.Position.Lat + dLat,
			West: n.Position.Lon - dLon, East: n.Position.Lon + dLon,
		})
	}
	// Merge until stable. A merged box can newly overlap a third, so one
	// pass is not enough; the count only ever shrinks, so it ends.
	for merged := true; merged; {
		merged = false
		for i := 0; i < len(boxes); i++ {
			for j := i + 1; j < len(boxes); j++ {
				if boxes[i].East < boxes[j].West || boxes[j].East < boxes[i].West ||
					boxes[i].North < boxes[j].South || boxes[j].North < boxes[i].South {
					continue
				}
				boxes[i] = llBox{
					South: math.Min(boxes[i].South, boxes[j].South),
					North: math.Max(boxes[i].North, boxes[j].North),
					West:  math.Min(boxes[i].West, boxes[j].West),
					East:  math.Max(boxes[i].East, boxes[j].East),
				}
				boxes = append(boxes[:j], boxes[j+1:]...)
				merged = true
				j--
			}
		}
	}
	return boxes, nil
}

func boxAreaKm2(b llBox) float64 {
	midLat := (b.South + b.North) / 2
	return (b.North - b.South) * 111.32 * (b.East - b.West) * 111.32 *
		math.Cos(midLat*math.Pi/180)
}

func patchesAreaKm2(patches []llBox) float64 {
	var a float64
	for _, b := range patches {
		a += boxAreaKm2(b)
	}
	return a
}

// environCacheDir is where one pull's tiles live: keyed by source and the
// patch set, so moving the network pulls fresh ground and reopening the same
// network reuses the cache without asking.
func environCacheDir(source string, patches []llBox) (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	keys := make([]string, 0, len(patches))
	for _, b := range patches {
		keys = append(keys, fmt.Sprintf("%.4f/%.4f/%.4f/%.4f", b.South, b.North, b.West, b.East))
	}
	sort.Strings(keys)
	sum := sha256.Sum256([]byte(strings.Join(keys, "|")))
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

// overpassNDJSON pulls OSM building ways in every patch, a chunk of patches
// per request so a national network is many small queries with progress
// rather than one enormous one. A way straddling two chunks would arrive
// twice - a building priced twice is a wall paid twice - so ways are
// deduplicated by id across the whole pull. Relations (multipolygons) are
// left to envgen: their outer rings need assembly this path does not
// attempt, and silently mangling them would be worse than saying so.
func overpassNDJSON(patches []llBox, progress func(done, total int)) (io.Reader, int, error) {
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
		resp, err := environDo("POST", "https://overpass-api.de/api/interpreter",
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

// fetchEnviron is the pull, run off the store's goroutine: resolve, download,
// ingest into the cache, then hand the directory to rf.environment exactly as
// if the operator had typed it.
func (s *Sim) fetchEnviron(source string, patches []llBox,
	progress func(done, total int)) (string, environ.IngestStats, error) {
	dir, err := environCacheDir(source, patches)
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
		if a := patchesAreaKm2(patches); a > overpassMaxKm2 {
			return "", environ.IngestStats{}, fmt.Errorf(
				"the node patches sum to %.0f km2, past the %.0f km2 a live "+
					"Overpass pull is fair for; prepare the region offline with tools/envgen",
				a, float64(overpassMaxKm2))
		}
		var n int
		rd, n, err = overpassNDJSON(patches, progress)
		if err == nil && n == 0 {
			err = fmt.Errorf("OpenStreetMap has no building ways in this map's area")
		}
	case "microsoft":
		files, uerr := microsoftFiles(patches)
		if uerr != nil {
			return "", environ.IngestStats{}, uerr
		}
		rd, done, err = microsoftNDJSON(files, patches,
			func(d int) { progress(d, len(files)+1) })
	case "merged":
		// The environment plan's actual shape: Microsoft for existence and
		// height, OSM tags for what it explicitly knows, explicit over
		// inferred. Both halves' caps apply, because both halves are pulled.
		if a := patchesAreaKm2(patches); a > overpassMaxKm2 {
			return "", environ.IngestStats{}, fmt.Errorf(
				"the node patches sum to %.0f km2, past the %.0f km2 a live "+
					"Overpass pull is fair for; prepare the region offline with tools/envgen",
				a, float64(overpassMaxKm2))
		}
		// Overpass first: it refuses fast when it refuses at all, and a
		// refusal after gigabytes of footprint downloads is a pull that
		// wasted an evening's bandwidth to fail.
		osm, _, oerr := overpassNDJSON(patches, func(done, total int) {
			progress(done, total+1)
		})
		if oerr != nil {
			return "", environ.IngestStats{}, oerr
		}
		files, uerr := microsoftFiles(patches)
		if uerr != nil {
			return "", environ.IngestStats{}, uerr
		}
		ms, msDone, merr := microsoftNDJSON(files, patches,
			func(d int) { progress(d, len(files)+1) })
		if merr != nil {
			return "", environ.IngestStats{}, merr
		}
		defer msDone()
		var mstats environ.MergeStats
		rd, mstats, err = environ.MergeGeoJSON(ms, osm)
		if err == nil && mstats.Primary+mstats.Enrich == 0 {
			err = fmt.Errorf("neither source has buildings in this map's area")
		}
	default:
		return "", environ.IngestStats{}, fmt.Errorf(
			"no building database %q; there is merged, osm and microsoft", source)
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
		patches, err := environPatches(s.nodes)
		if err != nil {
			return nil, err
		}
		const id = "environ-fetch"
		what := "buildings: " + source
		w.Jobs = append(w.Jobs, state.Job{ID: id, What: what, Total: 1})
		w.Say(fmt.Sprintf("pulling %s footprints: %d patch(es) around the "+
			"nodes, %.0f km2", source, len(patches), patchesAreaKm2(patches)))
		go func() {
			ctx := context.Background()
			dir, stats, err := s.fetchEnviron(source, patches,
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

	// environ.list: every pull already on disk, so switching environments
	// is a dropdown, not a re-download.
	st.Handle("environ.list", func(_ *state.World, _ any) (any, error) {
		// No cache directory and no pulls yet are both an empty list, not
		// an error: the dropdown's honest answer is "nothing downloaded".
		var root string
		if cache, err := os.UserCacheDir(); err == nil {
			root = filepath.Join(cache, "meshcoresim", "environment")
		}
		entries, _ := os.ReadDir(root)
		var dirs []string
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, filepath.Join(root, e.Name()))
			}
		}
		sort.Strings(dirs)
		return map[string]any{"dirs": dirs, "current": s.envDir}, nil
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
