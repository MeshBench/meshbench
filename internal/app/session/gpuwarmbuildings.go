// The environment's share of a GPU-warmed link. The kernel knows terrain;
// the roofs are priced here, with the same environ call the engine's own
// pathLoss makes, so the two ways a link gets priced cannot disagree.
package session

import (
	"math"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/MeshBench/meshbench/internal/rf/environ"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// warmJob is one uncontrolled pair the warm intends to measure.
type warmJob struct {
	a, b   int
	distKm float64
}

// warmPath is a job whose ground could be walked: the profile heights and
// the ends' heights above them.
type warmPath struct {
	idx     int
	heights []float32
	distM   float64
	aglA    float64
	aglB    float64
}

// gpuWarmDeadLossDB is where pricing buildings stops being worth the
// corridor walk: no budget on this network closes 175 dB, and a roof can
// only add. The cached number for such a pair differs from the engine's
// lazy fill by the building term, and both numbers say the same thing.
const gpuWarmDeadLossDB = 175

// priceBuildingsIntoWarm adds each surviving pair's building cost to the
// device's terrain losses, in parallel, with its own progress phase.
func priceBuildingsIntoWarm(env environ.Provider, g environ.Ground,
	nodes []scenario.Node, jobs []warmJob, results []warmPath, packedIdx []int,
	res []float32, freqMHz float64, workers int,
	progress func(what string, done, total int)) {
	if workers < 1 {
		workers = runtime.NumCPU()
	}
	// Indexed once over the fleet's extent, exactly as the engine now does:
	// the un-indexed corridor walk re-scanned tiles and allocated a fresh
	// seen-map per step per pair, and profiling a 451-node warm put 61% of
	// its half hour there. TestPathIndexMatchesDirect holds the crossings
	// equal, so this changes minutes, not decibels.
	ix := pathIndexOver(env, g, nodes)
	var priced int64
	var wg sync.WaitGroup
	next := 0
	var mu sync.Mutex
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				i := next
				next++
				mu.Unlock()
				if i >= len(packedIdx) {
					return
				}
				r := results[packedIdx[i]]
				j := jobs[packedIdx[i]]
				res[i] += float32(pairBuildingLossDB(ix,
					nodes[j.a], nodes[j.b], r.heights,
					r.aglA, r.aglB, r.distM, freqMHz, float64(res[i])))
				if c := atomic.AddInt64(&priced, 1); progress != nil &&
					(c == 1 || c%512 == 0) {
					progress("buildings along each link", int(c), len(packedIdx))
				}
			}
		}()
	}
	wg.Wait()
}

// pairBuildingLossDB is the environment's price for one warmed pair, from
// the profile the GPU judged - engine.pathLoss's own building term: the
// same arithmetic, through the same index.
func pairBuildingLossDB(ix *environ.PathIndex,
	na, nb scenario.Node, heights []float32,
	aglA, aglB, distM, freqMHz, terrainLossDB float64) float64 {
	if ix == nil || len(heights) < 2 || distM <= 0 || terrainLossDB > gpuWarmDeadLossDB {
		return 0
	}
	tx := float64(heights[0]) + aglA
	rx := float64(heights[len(heights)-1]) + aglB
	sc := envScratchPool.Get().(*environ.PathScratch)
	defer envScratchPool.Put(sc)
	return ix.PathLossDB(sc,
		na.Position.Lat, na.Position.Lon, tx,
		nb.Position.Lat, nb.Position.Lon, rx, distM, freqMHz)
}

// pathIndexOver builds the index for a fleet's extent, or nil for bare earth.
func pathIndexOver(env environ.Provider, g environ.Ground,
	nodes []scenario.Node) *environ.PathIndex {
	if env == nil || len(nodes) == 0 {
		return nil
	}
	south, north := nodes[0].Position.Lat, nodes[0].Position.Lat
	west, east := nodes[0].Position.Lon, nodes[0].Position.Lon
	for _, n := range nodes[1:] {
		south = math.Min(south, n.Position.Lat)
		north = math.Max(north, n.Position.Lat)
		west = math.Min(west, n.Position.Lon)
		east = math.Max(east, n.Position.Lon)
	}
	const marginDeg = 0.05
	return environ.NewPathIndex(env, g,
		south-marginDeg, west-marginDeg, north+marginDeg, east+marginDeg)
}

// envScratchPool hands each pricing worker its own epoch-marked scratch.
var envScratchPool = sync.Pool{New: func() any { return &environ.PathScratch{} }}
