// The tile cache: how much of it is held in memory, and where it lives on
// disk.
//
// Split from the GPU warm it shared a file with, which had reached the length
// limit. The two meet only at the tile store they both read.
package session

import (
	"context"
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
)

// tilesPerGB is how many decoded tiles a gigabyte holds: a tile is 256x256
// float32, a quarter of a megabyte.
const tilesPerGB = 4096

// registerTileCache is the tile cache bound, in the unit people think in, and
// where the cache lives on disk.
func registerTileCache(st *state.Store, s *Sim) {
	st.HandleSpec("terrain.cache", state.Spec{
		What: "say how much memory decoded terrain may occupy, in the unit " +
			"people think in, and read back where the tiles are kept and " +
			"whether this machine is allowed to fetch more",
		Params: []state.Param{
			{Name: "gb", Type: state.ParamNumber, Primary: true,
				What: "the ceiling in gigabytes, four thousand-odd tiles to the " +
					"gigabyte; anything under 0.25 is ignored rather than " +
					"refused, and absent only reports"},
		},
		Returns: []string{"gb", "dir", "downloads"},
		Answers: "`dir` is where the tiles live on disk, which is permanent: " +
			"nothing here expires a cached tile. `downloads` is whether terrain " +
			"may be fetched at all, answered here because this is the verb the " +
			"interface asks at startup and the switch has to draw its own " +
			"position on the first frame.",
		Example: &state.Example{
			Params: map[string]any{}, What: "ask where the terrain is and how much is held",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
		if v, ok := numField(p, "gb"); ok && v >= 0.25 {
			tiles := int(v * tilesPerGB)
			s.tileCacheTiles = tiles
			s.applyMemoryCeiling()
			if ts, ok := s.terr.(*terrain.TileStore); ok && ts != nil {
				ts.MaxLoadedTiles = tiles
			}
			w.TileCacheGB = v
			s.prefs.TileCacheGB = v
			_ = s.savePrefs(w)
			w.Say(fmt.Sprintf("the tile cache holds %.3g GB", v))
		}
		if w.TileCacheGB == 0 {
			if s.tileCacheTiles > 0 {
				w.TileCacheGB = float64(s.tileCacheTiles) / tilesPerGB
			} else {
				w.TileCacheGB = float64(terrain.DefaultMaxLoadedTiles) / tilesPerGB
			}
		}
		w.TileCacheDir = s.tileCacheDir()
		// Published here because this is the verb the interface asks at
		// startup, and the download switch has to be able to draw its own
		// position on the first frame rather than after somebody touches it.
		w.TerrainDownloads = s.terrainAllowed()
		return map[string]any{"gb": w.TileCacheGB, "dir": w.TileCacheDir,
			"downloads": w.TerrainDownloads}, nil
	})

	// Gigabytes of tiles, so it runs as a visible job on a worker, and the
	// store only swaps directories after the move has succeeded - the decoded
	// tiles in memory survive throughout.
	st.HandleSpec("terrain.cache_dir", state.Spec{
		What: "move the terrain cache to another disk, files and all, so a " +
			"permanent cache that has outgrown where it started does not have " +
			"to be downloaded again somewhere else",
		Params: []state.Param{
			{Name: "path", Type: state.ParamString, Primary: true,
				What: "where the cache should live, created if it is not there " +
					"already; absent only reports where it lives now, while a " +
					"path that cannot be written into, one that contains or sits " +
					"inside the current cache, or a second move while one is " +
					"still running, is refused rather than queued"},
		},
		Returns: []string{"moving", "to"},
		Answers: "the move runs on a worker with a progress job, so this " +
			"answers `moving` true before a byte has gone. Asked with no path it " +
			"answers `dir` instead. A move that fails leaves the cache where it " +
			"was and says so in the journal.",
		Example: &state.Example{
			Params:   "/srv/terrain",
			What:     "put the tiles on the disk with room for them",
			Runnable: false,
		},
	}, func(w *state.World, p any) (any, error) {
		path := soleString(p)
		if m, ok := p.(map[string]any); ok {
			if v, ok := m["path"].(string); ok {
				path = v
			}
		}
		if path == "" {
			return map[string]any{"dir": s.tileCacheDir()}, nil
		}
		oldDir := s.tileCacheDir()
		newDir, err := validateCacheDir(oldDir, path)
		if err != nil {
			return nil, err
		}
		if s.movingCache.Swap(true) {
			return nil, fmt.Errorf("a cache move is already running")
		}
		w.Say("moving the tile cache to " + newDir)
		go func() {
			defer s.movingCache.Store(false)
			ctx := context.Background()
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: "cachemove", What: "moving the tile cache"})
			n, err := moveTree(oldDir, newDir, func(done, total int) {
				_, _ = st.Do(ctx, "job.progress", state.Job{
					ID: "cachemove", What: "moving the tile cache",
					Done: done, Total: total})
			})
			_, _ = st.Do(ctx, "job.done", "cachemove")
			if err != nil {
				_, _ = st.Do(ctx, "ui.said", fmt.Sprintf(
					"the cache move stopped after %d files: %v - "+
						"the cache stays at %s", n, err, oldDir))
				return
			}
			_, _ = st.Do(ctx, "terrain.cache_moved",
				map[string]any{"dir": newDir, "files": n})
		}()
		return map[string]any{"moving": true, "to": newDir}, nil
	})

	// The store's goroutine is the only place the swap may happen.
	st.HandleInternalSpec("terrain.cache_moved", state.Spec{
		What: "point the tile store and the settings at the directory a " +
			"finished move has filled, and say whether the next launch " +
			"will look there too or download it all again",
		Params: []state.Param{
			{Name: "dir", Type: state.ParamString, Required: true,
				What: "the directory the files were moved to; the callback is " +
					"refused without it"},
			{Name: "files", Type: state.ParamNumber,
				What: "how many files moved, for the message; absent counts as none"},
		},
		Returns: []string{"dir"},
	}, func(w *state.World, p any) (any, error) {
		m, ok := p.(map[string]any)
		if !ok {
			return nil, wrongCallback("terrain.cache_moved")
		}
		dir, _ := m["dir"].(string)
		files := 0
		if v, ok := numField(p, "files"); ok {
			files = int(v)
		}
		if dir == "" {
			return nil, fmt.Errorf("terrain.cache_moved needs the directory")
		}
		if ts, ok := s.terr.(*terrain.TileStore); ok && ts != nil {
			ts.SetCacheDir(dir)
		}
		s.prefs.TileCacheDir = dir
		saved := s.savePrefs(w)
		w.TileCacheDir = dir
		// Where the tiles are is only half of it: a next launch that cannot
		// read the new location looks in the old one and downloads everything
		// again, so an unsaved move says so rather than promising twice.
		if saved == nil {
			w.Say(fmt.Sprintf("the tile cache lives at %s now - %d files moved, "+
				"nothing needs downloading again", dir, files))
		} else {
			w.Say(fmt.Sprintf("the tile cache lives at %s for this session - "+
				"%d files moved, and the next launch will look in the old place",
				dir, files))
		}
		return map[string]any{"dir": dir}, nil
	})
}
