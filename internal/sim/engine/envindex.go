// The environment, indexed once per engine.
//
// Pricing buildings through environ.PathBuildingLossDB walked the corridor
// with a fresh map and a bounding-box scan per step, per pair - and a link
// warm prices every pair. On a 451-node country it was 61% of the whole
// warm's CPU, most of that the runtime zeroing freshly allocated memory: the
// warm spent half an hour, and most of it was make(). The indexed twin -
// bucketed footprints, an epoch-marked scratch, no allocation on the hot
// path - existed the whole time; coverage rasters were using it. Only the
// engine was not.
//
// TestPathIndexMatchesDirect holds the two implementations to the same
// crossings, so this is a change of speed, not of physics.
package engine

import (
	"sync"

	"github.com/MeshBench/meshbench/internal/rf/environ"
)

// envIndex is the engine's building index, built lazily over the nodes'
// extent and rebuilt if the provider is swapped or the fleet grows - both
// are cheap checks against a rebuild that costs one pass over the dataset.
func (e *Engine) envIndex() *environ.PathIndex {
	e.mu.Lock()
	env := e.Env
	n := len(e.nodes)
	nodes := make([]*Node, n)
	copy(nodes, e.nodes)
	e.mu.Unlock()
	if env == nil {
		return nil
	}

	e.envMu.Lock()
	defer e.envMu.Unlock()
	if e.envIx != nil && e.envIxFor == env && e.envIxNodes == n {
		return e.envIx
	}
	if n == 0 {
		return nil
	}
	south, north := nodes[0].Spec().Position.Lat, nodes[0].Spec().Position.Lat
	west, east := nodes[0].Spec().Position.Lon, nodes[0].Spec().Position.Lon
	for _, nd := range nodes[1:] {
		p := nd.Spec().Position
		if p.Lat < south {
			south = p.Lat
		}
		if p.Lat > north {
			north = p.Lat
		}
		if p.Lon < west {
			west = p.Lon
		}
		if p.Lon > east {
			east = p.Lon
		}
	}
	// The corridor margin the un-indexed walk used, so a building just off
	// the line between two edge nodes is still indexed.
	const marginDeg = 0.05
	e.envIx = environ.NewPathIndex(env, e.Terrain,
		south-marginDeg, west-marginDeg, north+marginDeg, east+marginDeg)
	e.envIxFor, e.envIxNodes = env, n
	return e.envIx
}

// envScratchPool hands each worker its own epoch-marked scratch: the warm
// prices pairs from every core, and a shared scratch would be a data race
// where a pooled one is a free reuse.
var envScratchPool = sync.Pool{New: func() any { return &environ.PathScratch{} }}
