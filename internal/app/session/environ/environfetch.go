// Downloading building footprints at runtime.
//
// tools/envgen remains the way to prepare a region properly; this is the
// impatient path - pick a database in Configuration, pull what covers the
// loaded network, and test buildings without leaving the application. Only
// data crosses the network, the result is cached permanently like terrain,
// and a pull that would be enormous fails loudly rather than trying.
package environ

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	worldenv "github.com/MeshBench/meshbench/internal/rf/environ"
)

// microsoftMaxBytes caps a Microsoft pull by what the index says the files
// weigh. Disk is bounded separately - one file lives at a time - so this is
// about bandwidth and patience, and the failure message prices both.
const microsoftMaxBytes = 8e9

// fetchEnviron is the pull, run off the store's goroutine: resolve, download,
// ingest into the cache, then hand the directory to rf.environment exactly as
// if the operator had typed it.
func fetchEnviron(s *session.Sim, ctx context.Context, source string, patches []llBox,
	progress func(done, total int)) (string, worldenv.IngestStats, error) {
	dir, err := environCacheDir(source, patches)
	if err != nil {
		return "", worldenv.IngestStats{}, err
	}
	if hasTiles(dir) {
		return dir, worldenv.IngestStats{}, nil
	}
	var rd io.Reader
	var done func()
	switch source {
	case "osm":
		if a := patchesAreaKm2(patches); a > overpassMaxKm2 {
			return "", worldenv.IngestStats{}, fmt.Errorf(
				"the node patches sum to %.0f km2, past the %.0f km2 a live "+
					"Overpass pull is fair for. Narrow the network, or prepare the region offline: that needs tools/envgen from a source checkout, which a release bundle does not carry",
				a, float64(overpassMaxKm2))
		}
		var n int
		rd, n, err = overpassNDJSON(ctx, patches, progress)
		if err == nil && n == 0 {
			err = fmt.Errorf("OpenStreetMap has no building ways in this map's area")
		}
	case "microsoft":
		files, uerr := microsoftFiles(ctx, patches)
		if uerr != nil {
			return "", worldenv.IngestStats{}, uerr
		}
		rd, done, err = microsoftNDJSON(ctx, files, patches,
			func(d int) { progress(d, len(files)+1) })
	case "merged":
		// The environment plan's actual shape: Microsoft for existence and
		// height, OSM tags for what it explicitly knows, explicit over
		// inferred. Both halves' caps apply, because both halves are pulled.
		if a := patchesAreaKm2(patches); a > overpassMaxKm2 {
			return "", worldenv.IngestStats{}, fmt.Errorf(
				"the node patches sum to %.0f km2, past the %.0f km2 a live "+
					"Overpass pull is fair for. Narrow the network, or prepare the region offline: that needs tools/envgen from a source checkout, which a release bundle does not carry",
				a, float64(overpassMaxKm2))
		}
		// Overpass first: it refuses fast when it refuses at all, and a
		// refusal after gigabytes of footprint downloads is a pull that
		// wasted an evening's bandwidth to fail.
		osm, _, oerr := overpassNDJSON(ctx, patches, func(done, total int) {
			progress(done, total+1)
		})
		if oerr != nil {
			return "", worldenv.IngestStats{}, oerr
		}
		files, uerr := microsoftFiles(ctx, patches)
		if uerr != nil {
			return "", worldenv.IngestStats{}, uerr
		}
		ms, msDone, merr := microsoftNDJSON(ctx, files, patches,
			func(d int) { progress(d, len(files)+1) })
		if merr != nil {
			return "", worldenv.IngestStats{}, merr
		}
		defer msDone()
		var mstats worldenv.MergeStats
		rd, mstats, err = worldenv.MergeGeoJSON(ms, osm)
		if err == nil && mstats.Primary+mstats.Enrich == 0 {
			err = fmt.Errorf("neither source has buildings in this map's area")
		}
	default:
		return "", worldenv.IngestStats{}, fmt.Errorf(
			"no building database %q; there is merged, osm and microsoft", source)
	}
	if err != nil {
		return "", worldenv.IngestStats{}, err
	}
	if done != nil {
		defer done()
	}
	stats, err := worldenv.IngestGeoJSON(rd, dir, "uk")
	if err != nil {
		return "", stats, err
	}
	return dir, stats, nil
}

func registerEnvironFetch(st *state.Store, s *session.Sim) {
	// The heavy part runs as a job; what lands back on the store's goroutine
	// is only the outcome.
	st.Handle("environ.fetch", func(w *state.World, p any) (any, error) {
		source, _ := session.StringField(p, "source")
		if source == "" {
			source = "osm"
		}
		patches, err := environPatches(s.Nodes())
		if err != nil {
			return nil, err
		}
		const id = "environ-fetch"
		what := "buildings: " + source
		// A footprint pull is minutes of somebody else's bandwidth and had no
		// stop: the client's own 15-minute timeout was the only way out of one
		// started by accident. state.Job carries a Cancel and the jobs strip
		// draws it; this job simply never supplied one.
		ctx, stop := context.WithCancel(context.Background())
		w.Jobs = append(w.Jobs, state.Job{ID: id, What: what, Total: 1, Cancel: stop})
		w.Say(fmt.Sprintf("pulling %s footprints: %d patch(es) around the "+
			"nodes, %.0f km2", source, len(patches), patchesAreaKm2(patches)))
		go func() {
			defer stop()
			dir, stats, err := fetchEnviron(s, ctx, source, patches,
				func(done, total int) {
					_, _ = st.Do(ctx, "job.progress", state.Job{
						ID: id, What: what, Done: done, Total: total})
				})
			// Saying how it ended has to survive the thing that ended it.
			done, release := session.Finishing(ctx)
			defer release()
			if err != nil {
				// A stop is not a failure. Reporting one as the other teaches
				// an operator to distrust the button they just pressed - the
				// tile fetch learned this first.
				if ctx.Err() != nil {
					_, _ = st.Do(done, "ui.said",
						"the "+source+" footprint pull was stopped; "+
							"what had already been written is cached")
					return
				}
				_, _ = st.Do(done, "environ.failed", err.Error())
				return
			}
			note := "already cached"
			if stats.Buildings > 0 {
				note = fmt.Sprintf("%d building(s) into %d tile(s), %d skipped",
					stats.Buildings, stats.Tiles, stats.Skipped)
			}
			_, _ = st.Do(done, "environ.fetched", note)
			// The same verb the manual path uses, so there is exactly one
			// way buildings get switched on.
			_, _ = st.Do(done, "rf.environment", map[string]any{"dir": dir})
		}()
		return map[string]any{"source": source, "started": true}, nil
	})

	st.Handle("environ.list", func(_ *state.World, _ any) (any, error) {
		// No cache directory and no pulls yet are both an empty list, not
		// an error: the dropdown's honest answer is "nothing downloaded".
		var root string
		if cache, err := os.UserCacheDir(); err == nil {
			root = filepath.Join(cache, "meshbench", "environment")
		}
		entries, _ := os.ReadDir(root)
		var dirs []string
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, filepath.Join(root, e.Name()))
			}
		}
		sort.Strings(dirs)
		return map[string]any{"dirs": dirs, "current": s.EnvDir()}, nil
	})

	// The jobs and the journal belong to the store's goroutine, so the worker
	// cannot close its own job.
	st.HandleInternal("environ.fetched", func(w *state.World, p any) (any, error) {
		w.Jobs = session.FinishJob(w.Jobs, "environ-fetch")
		w.Say("footprints ready: " + session.PrimaryString(p, "note"))
		return nil, nil
	})

	st.HandleInternal("environ.failed", func(w *state.World, p any) (any, error) {
		w.Jobs = session.FinishJob(w.Jobs, "environ-fetch")
		w.Say("building pull failed: " + session.PrimaryString(p, "reason"))
		return nil, nil
	})
}
