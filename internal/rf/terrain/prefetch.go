// A bulk download: what it will cost before it starts, and moving it.
//
// Split from the store itself, which is about one tile: answering an elevation
// query, fetching that tile if it has to, and keeping what it decoded. This
// file is about the whole download instead - the box or the list, priced, then
// carried on eight connections with progress. The two meet only at get() and
// path(), and they are asked by different callers for different reasons: the
// store is asked by a profile walk that wants a height, and this is asked by
// somebody deciding whether to spend half a gigabyte.
package terrain

import (
	"context"
	"os"
	"sync"
)

// TilesForBounds counts the tiles a bounding box needs, which is what an
// estimate is made of: a user asked to wait should be told for how long and for
// how much, before it starts rather than after.
func TilesForBounds(south, north, west, east float64, zoom int) (count int, x0, y0, x1, y1 int) {
	x0, y1 = TileXY(south, west, zoom)
	x1, y0 = TileXY(north, east, zoom)
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	return (x1 - x0 + 1) * (y1 - y0 + 1), x0, y0, x1, y1
}

// Estimate describes what covering a box will cost.
type Estimate struct {
	Tiles      int
	Cached     int
	ToFetch    int
	BytesRough int64
}

// Estimate reports the download before it happens.
//
// The figure is approximate and says so in its name. It exists so a UI can say
// "412 tiles, about 25 MB" and let someone on a tethered connection decline.
func (s *TileStore) Estimate(south, north, west, east float64) Estimate {
	count, x0, y0, x1, y1 := TilesForBounds(south, north, west, east, s.zoom())
	e := Estimate{Tiles: count}
	for x := x0; x <= x1; x++ {
		for y := y0; y <= y1; y++ {
			if _, err := os.Stat(s.path(x, y)); err == nil {
				e.Cached++
			}
		}
	}
	e.ToFetch = e.Tiles - e.Cached
	e.BytesRough = int64(e.ToFetch) * roughBytesPerTile
	return e
}

// roughBytesPerTile is the average terrarium tile, near enough to price a
// download before it happens and never near enough to report as a measurement.
//
// Measured rather than remembered, and it errs high on purpose. The figure
// here was 60 kB, which quoted 365 MB for the ground under the Scotland and
// Ireland network; 6,233 tiles later that cache held 525 MB, an average of
// 82 kB. A quote somebody decides on has to be wrong in the direction that
// costs them nothing: an estimate that flatters is how a metered connection
// agrees to half again as much as it was told.
const roughBytesPerTile = 82 << 10

// EstimateTiles prices an explicit tile list, the shape PrefetchTiles takes.
//
// The list form exists for the same reason PrefetchTiles does: the tiles under
// a network's links are a fraction of the box around it, so pricing the box
// quotes a figure several times the real one and would have somebody decline a
// download they could easily afford.
func (s *TileStore) EstimateTiles(tiles [][2]int) Estimate {
	e := Estimate{Tiles: len(tiles)}
	for _, t := range tiles {
		if _, err := os.Stat(s.path(t[0], t[1])); err == nil {
			e.Cached++
		}
	}
	e.ToFetch = e.Tiles - e.Cached
	e.BytesRough = int64(e.ToFetch) * roughBytesPerTile
	return e
}

// Prefetch downloads what a box is missing, reporting progress over exactly
// that: the tiles it will actually move.
//
// Done up front rather than lazily during a computation because a raster that
// stalls for a minute in the middle looks like a hang, and because the total is
// only knowable in advance. Two properties are deliberate. Cached tiles are
// skipped by a stat, never by get(): getting a cached tile decodes it into
// memory, so a prefetch over an already-cached country cost gigabytes of RAM
// to confirm what the filesystem already knew. And the missing ones download
// on several connections at once - one at a time, a few thousand tiles of a
// fresh region is most of an hour spent watching one request's latency.
func (s *TileStore) Prefetch(ctx context.Context, south, north, west, east float64) error {
	_, x0, y0, x1, y1 := TilesForBounds(south, north, west, east, s.zoom())
	tiles := make([][2]int, 0, (x1-x0+1)*(y1-y0+1))
	for x := x0; x <= x1; x++ {
		for y := y0; y <= y1; y++ {
			tiles = append(tiles, [2]int{x, y})
		}
	}
	return s.PrefetchTiles(ctx, tiles)
}

// PrefetchTiles downloads whichever of the given tiles are not on disk yet.
//
// The list form exists because a bounding box is usually the wrong shape: a
// link warm samples terrain only under the lines between its nodes, and the
// rectangle around a coastal country is mostly open sea those lines never
// cross. Prefetching the box fetched twenty-eight thousand tiles of Atlantic
// while the operator watched the warm sit on zero percent.
func (s *TileStore) PrefetchTiles(ctx context.Context, tiles [][2]int) error {
	var missing [][2]int
	for _, t := range tiles {
		if _, err := os.Stat(s.path(t[0], t[1])); err != nil {
			missing = append(missing, t)
		}
	}
	if s.OnProgress != nil {
		s.OnProgress(0, len(missing))
	}
	if len(missing) == 0 {
		return nil
	}

	// Bounded fan-out: kind to the tile host, decisive against latency. The
	// first error stops the hand-out - a host that refused one tile is about
	// to refuse the rest, and a thousand further failures say nothing new.
	const fetchers = 8
	jobs := make(chan [2]int)
	var wg sync.WaitGroup
	// One voice for progress: the callback's contract predates the fan-out,
	// and every existing caller writes plain state from it. Serialised here,
	// the count also stays monotonic instead of arriving shuffled.
	var progressMu sync.Mutex
	done := 0
	var firstErr error
	var errOnce sync.Once
	fetchCtx, stop := context.WithCancel(ctx)
	defer stop()
	for w := 0; w < fetchers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range jobs {
				if fetchCtx.Err() != nil {
					continue
				}
				if _, err := s.get(fetchCtx, t[0], t[1]); err != nil {
					errOnce.Do(func() { firstErr = err; stop() })
					continue
				}
				progressMu.Lock()
				done++
				if s.OnProgress != nil {
					s.OnProgress(done, len(missing))
				}
				progressMu.Unlock()
			}
		}()
	}
	for _, t := range missing {
		if fetchCtx.Err() != nil {
			break
		}
		jobs <- t
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}
