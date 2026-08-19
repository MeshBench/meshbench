// The environment's share of a GPU-warmed link. The kernel knows terrain;
// the roofs are priced here, with the same environ call the engine's own
// pathLoss makes, so the two ways a link gets priced cannot disagree.
package session

import (
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/MeshBench/meshbench/internal/environ"
	"github.com/MeshBench/meshbench/internal/scenario"
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
				res[i] += float32(pairBuildingLossDB(env, g,
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
// same environ call, the same endpoint construction.
func pairBuildingLossDB(env environ.Provider, g environ.Ground,
	na, nb scenario.Node, heights []float32,
	aglA, aglB, distM, freqMHz, terrainLossDB float64) float64 {
	if env == nil || len(heights) < 2 || distM <= 0 || terrainLossDB > gpuWarmDeadLossDB {
		return 0
	}
	tx := float64(heights[0]) + aglA
	rx := float64(heights[len(heights)-1]) + aglB
	return environ.PathBuildingLossDB(env, g,
		na.Position.Lat, na.Position.Lon, tx,
		nb.Position.Lat, nb.Position.Lon, rx, distM, freqMHz)
}
