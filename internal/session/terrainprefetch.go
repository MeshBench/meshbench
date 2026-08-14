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

	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/terrain"
)

func registerTerrainPrefetch(st *state.Store, s *Sim) {
	st.Handle("terrain.prefetch", func(w *state.World, _ any) (any, error) {
		if len(w.Nodes) == 0 {
			return nil, fmt.Errorf("no network loaded, so no area to fetch")
		}
		ts, ok := s.terrain().(*terrain.TileStore)
		if !ok || ts == nil {
			return nil, fmt.Errorf("no tile store on this machine")
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
		go func() {
			defer s.prefetching.Store(false)
			ctx := context.Background()
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: "tiles", What: "fetching terrain tiles", Total: est.Tiles})
			ts.OnProgress = func(done, total int) {
				if done == 1 || done%16 == 0 || done == total {
					_, _ = st.Do(ctx, "job.progress", state.Job{
						ID: "tiles", What: "fetching terrain tiles",
						Done: done, Total: total})
				}
			}
			err := ts.Prefetch(ctx, south, north, west, east)
			ts.OnProgress = nil
			_, _ = st.Do(ctx, "job.done", "tiles")
			if err != nil {
				_, _ = st.Do(ctx, "ui.said", "the tile fetch stopped: "+err.Error())
				return
			}
			_, _ = st.Do(ctx, "ui.said", fmt.Sprintf(
				"the ground under this study is cached: %d tiles", est.Tiles))
		}()
		return map[string]any{"tiles": est.Tiles, "to_fetch": est.ToFetch,
			"bytes_rough": est.BytesRough}, nil
	})
}
