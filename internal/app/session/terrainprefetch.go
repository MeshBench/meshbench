// Prefetching terrain, visibly.
//
// The tiles under a study area download lazily, inside whatever computation
// first wants them - which on a fresh machine puts minutes of network time in
// the middle of a warm, invisibly. This makes it a choice: say what the
// download would cost, then do it as a job with progress.
package session

import (
	"context"
	"fmt"
	"math"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/dsp"
	"github.com/MeshBench/meshbench/internal/rf/geo"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// profileTiles is every tile a warm's profiles can sample: the union of the
// tiles under each pair's line, with each step's neighbours for the bilinear
// reads at tile edges. On a country this is a few tens of thousands of
// entries against a bounding box's hundreds of thousands, and none of them
// are sea nobody will ever ask about.
func profileTiles(nodes []scenario.Node, zoom int) [][2]int {
	if zoom <= 0 {
		zoom = terrain.DefaultZoom
	}
	// Half a tile of longitude per step, latitude's way; oversampling only
	// costs set inserts.
	stepDeg := 360 / math.Exp2(float64(zoom)) / 2
	seen := map[[2]int]bool{}
	var out [][2]int
	add := func(x, y int) {
		for _, d := range [5][2]int{{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			k := [2]int{x + d[0], y + d[1]}
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	// Only the pairs a warm will actually walk: the same free-space and
	// Earth-bulge cull the engine and the device apply, with the same
	// generous gain allowance, so no tile is fetched for a pair physics has
	// already refused. This is the difference between the ground under a
	// country's links and the Sahara under one stray equatorial node.
	noise := dsp.NoiseFloorDBm(250e3, 6)
	for i := range nodes {
		for j := i + 1; j < len(nodes); j++ {
			a, b := nodes[i].Position, nodes[j].Position
			distKm := geo.DistanceKm(a.Lat, a.Lon, b.Lat, b.Lon)
			if distKm <= 0 {
				continue
			}
			fspl := terrain.FSPLdB(distKm, 869.525)
			bulge := terrain.EarthBulgeLossDB(distKm*1000,
				nodes[i].HeightAGLm, nodes[j].HeightAGLm, 869.525)
			bestTx := math.Max(nodes[i].TxPowerDBm, nodes[j].TxPowerDBm)
			if bestTx+24-fspl-bulge < noise-30 {
				continue
			}
			span := math.Max(math.Abs(b.Lat-a.Lat), math.Abs(b.Lon-a.Lon))
			steps := int(span/stepDeg) + 1
			for k := 0; k <= steps; k++ {
				f := float64(k) / float64(steps)
				x, y := terrain.TileXY(a.Lat+(b.Lat-a.Lat)*f, a.Lon+(b.Lon-a.Lon)*f, zoom)
				add(x, y)
			}
		}
	}
	return out
}

// terrainWords is what a terrain download calls itself while it runs.
//
// It names the network as the thing taking the time and prices it in the unit
// the operator is charged in. The line used to say "measuring every link"
// throughout, because the measurement's job was the one on screen and the
// download's was not, so the first minutes of a fresh install were spent
// watching a percentage crawl near zero with no mention anywhere that half a
// gigabyte was arriving. The total is "about" because it is an average tile
// size times a count; the megabytes already spent are measured.
func terrainWords(got, rough int64) string {
	const mb = 1 << 20
	if rough <= 0 {
		return fmt.Sprintf("fetching terrain, %d MB so far", got/mb)
	}
	// An estimate this download has already passed is an estimate, not a
	// total, and printing "390 MB of about 365 MB" reads as a fault in the
	// arithmetic rather than as the approximation it was always labelled.
	if got > rough {
		return fmt.Sprintf("fetching terrain, %d MB so far, past the %d MB estimated",
			got/mb, rough/mb)
	}
	return fmt.Sprintf("fetching terrain, %d MB of about %d MB", got/mb, rough/mb)
}

func registerTerrainPrefetch(st *state.Store, s *Sim) {
	st.HandleSpec("terrain.prefetch", state.Spec{
		What: "download the ground under the loaded network before anything " +
			"needs it, so the minutes of network time a first measurement " +
			"would spend invisibly are a visible, priced, stoppable job instead",
		Returns: []string{"tiles", "to_fetch", "bytes_rough"},
		Answers: "`tiles` is the area's whole tile count and `to_fetch` how " +
			"many of them are missing, so `to_fetch` zero means the ground is " +
			"already cached and nothing was started. Otherwise the download runs " +
			"as a cancellable job and this returns before it, with " +
			"`bytes_rough` an average tile size times a count rather than a " +
			"measurement. Refused where no network is loaded, where the machine " +
			"has no tile store, and where terrain downloads are switched off, " +
			"which terrain.allow is what turns on.",
		Example: &state.Example{
			Params: map[string]any{}, What: "fetch the ground this study stands on",
			Runnable: false,
		},
	}, func(w *state.World, _ any) (any, error) {
		if len(w.Nodes) == 0 {
			return nil, fmt.Errorf("no network loaded, so no area to fetch")
		}
		ts, ok := s.terrain().(*terrain.TileStore)
		if !ok || ts == nil {
			return nil, fmt.Errorf("no tile store on this machine")
		}
		// One gate, named where it is: a prefetch that silently granted its
		// own permission would be the way around the only question the
		// application asks before spending somebody's bandwidth.
		if ts.Offline {
			return nil, fmt.Errorf(
				"terrain downloads are off on this machine; terrain.allow turns them on")
		}
		south, north := math.Inf(1), math.Inf(-1)
		west, east := math.Inf(1), math.Inf(-1)
		for i := range w.Nodes {
			south = math.Min(south, w.Nodes[i].Lat)
			north = math.Max(north, w.Nodes[i].Lat)
			west = math.Min(west, w.Nodes[i].Lon)
			east = math.Max(east, w.Nodes[i].Lon)
		}
		// The margin, in degrees, latitude's way: approximate is fine for a
		// download bound, and a tile too many costs 60 kB.
		margin := w.MarginKm / 111
		south, north = south-margin, north+margin
		west, east = west-margin, east+margin

		est := ts.Estimate(south, north, west, east)
		if est.ToFetch == 0 {
			w.Say(fmt.Sprintf("all %d tiles for this area are already cached", est.Tiles))
			return map[string]any{"tiles": est.Tiles, "to_fetch": 0}, nil
		}
		if s.prefetching.Swap(true) {
			return nil, fmt.Errorf("a prefetch is already running")
		}
		w.Say(fmt.Sprintf("fetching %d of %d tiles, roughly %d MB",
			est.ToFetch, est.Tiles, est.BytesRough>>20))
		// Cancellable, and the handle registered with the job rather than
		// kept here: a download of a few thousand tiles is the longest thing
		// an operator can start by accident, and the only honest way to offer
		// a stop is to hand the store the function that performs it.
		ctx, stop := context.WithCancel(context.Background())
		go func() {
			defer s.prefetching.Store(false)
			defer stop()
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: "tiles", What: terrainWords(0, est.BytesRough),
				Total: est.ToFetch, Cancel: stop})
			start := ts.FetchedBytes()
			ts.OnProgress = func(done, total int) {
				if done == 1 || done%16 == 0 || done == total {
					_, _ = st.Do(ctx, "job.progress", state.Job{
						ID: "tiles", What: terrainWords(ts.FetchedBytes()-start, est.BytesRough),
						Done: done, Total: total})
				}
			}
			err := ts.Prefetch(ctx, south, north, west, east)
			ts.OnProgress = nil
			// Outliving the cancel on purpose: the stop is the moment the
			// operator most needs to be told what happened.
			done, release := finishing(ctx)
			defer release()
			_, _ = st.Do(done, "job.done", "tiles")
			if err != nil {
				// A cancel is not a failure, and saying so as one would teach
				// an operator to distrust the button they just pressed.
				if ctx.Err() != nil {
					_, _ = st.Do(done, "ui.said",
						"the tile fetch was stopped; what had already arrived is cached")
					return
				}
				_, _ = st.Do(done, "ui.said", "the tile fetch stopped: "+err.Error())
				return
			}
			_, _ = st.Do(done, "ui.said", fmt.Sprintf(
				"the ground under this study is cached: %d tiles", est.Tiles))
		}()
		return map[string]any{"tiles": est.Tiles, "to_fetch": est.ToFetch,
			"bytes_rough": est.BytesRough}, nil
	})
}

// prefetchWarmTerrain fills in the ground a walk is about to sample, before
// any profile can stall on a download.
//
// The lazy path still works - a missing tile fetches the moment a profile
// first touches it - but it works invisibly, one tile at a time, from the
// middle of the measurement: on a fresh region that is minutes of network
// latency smeared through a warm that looks hung. This says the cost first,
// fetches it as a job with a percentage, and only then lets the walking
// begin. Nothing to fetch is the common case and stays silent.
func (s *Sim) prefetchWarmTerrain(ctx context.Context, st *state.Store, nodes []scenario.Node) {
	ts, ok := s.terrain().(*terrain.TileStore)
	if !ok || ts == nil || ts.Offline || len(nodes) == 0 {
		return
	}
	if s.prefetching.Swap(true) {
		// The operator's own prefetch is running; whatever it misses, the
		// walk fetches lazily as before.
		return
	}
	defer s.prefetching.Store(false)
	// The tiles under the lines, not the box around the fleet. Profiles
	// sample terrain only between pairs of nodes, and the rectangle around a
	// coastal country is mostly open sea no profile crosses - prefetching the
	// box fetched twenty-eight thousand tiles of Atlantic while the operator
	// watched the warm sit on zero percent. Each line is walked at half-tile
	// steps with its neighbours taken along for the bilinear reads at edges.
	tiles := profileTiles(nodes, ts.Zoom)
	// Progress totals are unknowable before the stat pass, so the job opens
	// with what is being decided and re-opens with the real count the moment
	// PrefetchTiles has one.
	what := "checking the ground under every link"
	// Stoppable, unlike before. This is the longest thing a launch starts on
	// its own, and a download somebody wants to get out of should not need the
	// window closing to do it. Stopping leaves the warm running on whatever
	// has already landed.
	fetchCtx, stop := context.WithCancel(ctx)
	defer stop()
	_, _ = st.Do(ctx, "job.progress", state.Job{
		ID: "tiles", What: what, Total: 1, Cancel: stop})
	// Megabytes as well as tiles, and measured ones: the words used to name
	// only the tile count, so the line that owned the first minutes of a fresh
	// install never once said how much of somebody's connection it was
	// spending. The tiles stay as the numerator because that percentage is
	// exact, where the total in bytes can only ever be an average times a
	// count. Reported per sixteen tiles, which is a whole percent of a
	// country's worth.
	rough := ts.EstimateTiles(tiles).BytesRough
	// Cumulative over the store's whole life, so this warm's share is the
	// difference. Without the baseline a second region opened in one session
	// began its download reporting the first one's megabytes.
	start := ts.FetchedBytes()
	ts.OnProgress = func(done, total int) {
		if total == 0 {
			return
		}
		if done == 0 || done == 1 || done%16 == 0 || done == total {
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: "tiles", What: terrainWords(ts.FetchedBytes()-start, rough),
				Done: done, Total: total})
		}
	}
	err := ts.PrefetchTiles(fetchCtx, tiles)
	ts.OnProgress = nil
	done, release := finishing(ctx)
	defer release()
	_, _ = st.Do(done, "job.done", "tiles")
	if err == nil || ctx.Err() != nil {
		return
	}
	if fetchCtx.Err() != nil {
		// A stop is not a failure, and saying so as one would teach an
		// operator to distrust the button they just pressed.
		_, _ = st.Do(done, "ui.said",
			"the terrain fetch was stopped; what had already arrived is cached")
		return
	}
	// Reported, then out of the way: the walk's own lazy fetch and its
	// honest no-data misses take over from here.
	_, _ = st.Do(done, "ui.said",
		"the terrain fetch stopped: "+err.Error()+
			" - the walk will fetch what it can as it goes")
}
