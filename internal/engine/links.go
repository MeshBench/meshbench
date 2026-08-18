// What every pair of nodes costs each other, and the cache that keeps it
// affordable.
//
// Path loss is the expensive thing the engine does: a terrain profile per
// pair, n-squared over the network. Everything here exists because that has
// to be computed once and then not again - primed from the GPU, warmed on
// the cores, snapshotted across a rebuild, and invalidated only when a node
// actually moves.
package engine

import (
	"context"
	"math"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/MeshBench/meshbench/internal/dsp"
	"github.com/MeshBench/meshbench/internal/environ"
	"github.com/MeshBench/meshbench/internal/scenario"
	"github.com/MeshBench/meshbench/internal/terrain"
)

// pathLoss is free-space plus terrain diffraction between two nodes, cached.
func (e *Engine) pathLoss(a, b int) (float64, bool) {
	if a > b {
		a, b = b, a
	}
	k := [2]int{a, b}

	e.mu.Lock()
	if v, ok := e.linkCache[k]; ok {
		e.mu.Unlock()
		if math.IsInf(v, 1) {
			return 0, false
		}
		return v, true
	}
	from, to := e.nodes[a].Spec, e.nodes[b].Spec
	e.mu.Unlock()

	distKm := haversineKm(from.Position.Lat, from.Position.Lon, to.Position.Lat, to.Position.Lon)
	if distKm <= 0 {
		return 0, false
	}

	// The free-space cull. Terrain can only ever add loss, so if the pair
	// cannot matter even over flat vacuum — not as a signal, not as
	// interference — there is nothing a terrain profile could change, and the
	// profile is the expensive part. On a country-sized import most pairs are
	// like this, and profiling them anyway is what turned the first flood on a
	// 300-node scenario into a frozen minute: the lazy cache fill walked
	// forty-five thousand pairs of DEM samples on the frame thread.
	fspl := terrain.FSPLdB(distKm, e.phyOf(from).freqMHz)
	bestTx := math.Max(from.TxPowerDBm, to.TxPowerDBm)
	bestRx := bestTx + gain(from) + gain(to) - fspl
	// The quieter of the two receivers. This is a cull, so the question is
	// whether *either* end could hear the other: taking the worse figure would
	// discard a pair the better receiver can close.
	noise := dsp.NoiseFloorDBm(e.phyOf(from).bandwidthHz,
		math.Min(e.noiseFigOf(from), e.noiseFigOf(to)))
	if bestRx < noise-30 {
		e.mu.Lock()
		e.linkCache[k] = fspl // an underestimate, and irrelevant below the floor
		e.mu.Unlock()
		return fspl, true
	}

	// The expensive branch, counted so a caller mid-run can say why it just
	// paused: this is what a warm exists to do off the delivery path, so
	// landing here during play means some pair the last warm measured is not
	// the pair this tick needs - most often a node's radio reporting a real
	// configuration the warm ran before it had.
	e.liveProfiles.Add(1)
	profile, ok := e.profile(from, to, distKm)
	loss := math.Inf(1)
	if ok {
		loss = fspl +
			terrain.MultiEdgeLossDB(profile, from.HeightAGLm, to.HeightAGLm, e.phyOf(from).freqMHz) +
			e.buildingLossDB(from, to, profile) +
			e.Config.ExcessPathLossDB
	}

	e.mu.Lock()
	e.linkCache[k] = loss
	e.mu.Unlock()
	if !ok {
		return 0, false
	}
	return loss, true
}

func (e *Engine) profile(from, to scenario.Node, distKm float64) ([]terrain.Point, bool) {
	n := int(distKm * 1000 / e.Config.ProfileStepM)
	if n < 2 {
		n = 2
	}
	if n > 256 {
		n = 256
	}
	out := make([]terrain.Point, n+1)
	for i := 0; i <= n; i++ {
		f := float64(i) / float64(n)
		h, ok := e.Terrain.ElevationM(
			from.Position.Lat+(to.Position.Lat-from.Position.Lat)*f,
			from.Position.Lon+(to.Position.Lon-from.Position.Lon)*f)
		if !ok {
			return nil, false
		}
		out[i] = terrain.Point{DistM: f * distKm * 1000, HeightM: h}
	}
	return out, true
}

// LiveProfiles is how many pairs have been profiled outside a warm, this
// engine's whole lifetime. Cumulative rather than reset per tick, because the
// caller only wants to know it moved since it last looked.
func (e *Engine) LiveProfiles() int64 {
	return e.liveProfiles.Load()
}

// LinkCacheSnapshot copies the measured matrix out, so a rebuild that changes
// nothing geometric does not have to measure it again.
func (e *Engine) LinkCacheSnapshot() map[[2]int]float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[[2]int]float64, len(e.linkCache))
	for k, v := range e.linkCache {
		out[k] = v
	}
	return out
}

// RestoreLinkCache puts a snapshot back. The caller vouches that the geometry
// it was measured over is this engine's geometry; nothing here can check that,
// which is why the session keys it on a hash of everything the loss depends
// on.
func (e *Engine) RestoreLinkCache(m map[[2]int]float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for k, v := range m {
		e.linkCache[k] = v
	}
}

// PrimeLinks fills the cache from a matrix somebody else computed.
//
// The matrix is the upper triangle of an n by n grid, in the same node order
// the engine holds, carrying free-space plus diffraction and not the excess
// path loss, which is this engine's own setting and is added here. Pairs the
// terrain could not answer for are left out rather than guessed at: they fall
// back to the profile the lazy path would have taken.
//
// It exists so the measurement can be done somewhere other than these cores -
// on a GPU, where forty-eight thousand independent profiles is what the
// hardware is for - without that path having to know anything about the
// engine's locking or its cache.
func (e *Engine) PrimeLinks(n int, loss []float32, noData float32) int {
	if n <= 1 || len(loss) < n*n {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if n != len(e.nodes) {
		// A matrix about a different network is worse than no matrix.
		return 0
	}
	filled := 0
	for a := 0; a < n; a++ {
		for b := a + 1; b < n; b++ {
			v := loss[a*n+b]
			if v == noData || math.IsInf(float64(v), 0) || math.IsNaN(float64(v)) {
				continue
			}
			e.linkCache[[2]int{a, b}] = float64(v) + e.Config.ExcessPathLossDB
			filled++
		}
	}
	return filled
}

// WarmLinks computes the whole path-loss matrix, in parallel.
//
// The cache fills lazily otherwise, which means the first flood pays for every
// pair at once, on whatever thread sent the message. Warming does the same
// work where it belongs: up front, across every core, with a progress figure
// someone can watch. Safe alongside a running engine — pathLoss is locked, and
// a pair warmed twice costs one map hit.
func (e *Engine) WarmLinks(ctx context.Context, progress func(done, total int)) {
	e.mu.Lock()
	n := len(e.nodes)
	e.mu.Unlock()

	type pair struct{ a, b int }
	pairs := make(chan pair, 256)
	go func() {
		defer close(pairs)
		for a := 0; a < n; a++ {
			for b := a + 1; b < n; b++ {
				select {
				case pairs <- pair{a, b}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	total := n * (n - 1) / 2
	var done atomic.Int64
	workers := runtime.NumCPU()
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range pairs {
				e.pathLoss(p.a, p.b)
				// The first pair as well as every 512th: the first is what
				// moves a status line off the previous phase, and it is the
				// one a throttle otherwise never lets through.
				if d := done.Add(1); progress != nil && (d == 1 || d%512 == 0) {
					progress(int(d), total)
				}
			}
		}()
	}
	wg.Wait()
	if progress != nil {
		progress(int(done.Load()), total)
	}
}

// InvalidateLinks drops the path-loss cache.
//
// A node that moved changes every path it is part of, and a cache keyed on node
// index cannot know which. Dropping all of it is cheaper than being clever, and
// the matrix rewarms in the background.
func (e *Engine) InvalidateLinks() {
	e.mu.Lock()
	e.linkCache = map[[2]int]float64{}
	e.emitterNoise = map[int]float64{}
	e.mu.Unlock()
}

// PathLossForTest exposes the cached link for measurements and tests.
func (e *Engine) PathLossForTest(a, b int) (float64, bool) { return e.pathLoss(a, b) }

// buildingLossDB is what the buildings along a path cost it, when an
// environment is loaded: knife-edge diffraction over each rooftop the
// profile now has to clear, plus one wall's worth of material loss per
// building the direct ray actually passes through. The dataset supplied
// what stands there; the pricing happens here, at this run's frequency -
// the environment plan's core rule, kept.
func (e *Engine) buildingLossDB(from, to scenario.Node, profile []terrain.Point) float64 {
	if e.Env == nil || len(profile) < 2 {
		return 0
	}
	obs := environ.ObstructionsOnPath(e.Env, e.Terrain,
		from.Position.Lat, from.Position.Lon, to.Position.Lat, to.Position.Lon)
	if len(obs) == 0 {
		return 0
	}
	total := profile[len(profile)-1].DistM
	// The antenna heights the direct ray runs between, above sea level.
	txM := profile[0].HeightM + from.HeightAGLm
	rxM := profile[len(profile)-1].HeightM + to.HeightAGLm

	loss := 0.0
	freq := e.phyOf(from).freqMHz
	for _, o := range obs {
		midFrac := (o.EnterFrac + o.ExitFrac) / 2
		rayM := txM + (rxM-txM)*midFrac
		// The rooftop as a knife edge at its position along the path - the
		// same ITU-R P.526 arithmetic terrain uses, so a building and a
		// ridge of equal height cost the same, as they should.
		d1 := total * midFrac
		d2 := total - d1
		if d1 <= 0 || d2 <= 0 {
			continue
		}
		h := o.TopM - (txM + (rxM-txM)*midFrac)
		v := terrain.FresnelParameter(h, d1, d2, freq)
		loss += terrain.KnifeEdgeDB(v)
		// And the walls, when the ray goes through rather than over.
		if o.TopM > rayM {
			loss += environ.MaterialLossDB(o.Material, freq)
		}
	}
	return loss
}

// DropLinkCache forgets every measured path, for when the physics that
// priced them changed - an environment loading, most of all. The next warm
// or delivery re-measures under the new rules.
func (e *Engine) DropLinkCache() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.linkCache = map[[2]int]float64{}
}
