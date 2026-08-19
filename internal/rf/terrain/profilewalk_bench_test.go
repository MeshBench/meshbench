package terrain

import (
	"math/rand"
	"os"
	"testing"
)

// The link warm's actual workload, against a real tile store: many long
// profiles across a country, each up to 257 bilinear samples. Run it with
//
//	MESHBENCH_TERRAIN_DIR=~/.cache/meshcoresim/terrain go test -bench ProfileWalk
//
// against a populated cache; without the variable it skips, because a
// benchmark that fetches tiles from the network measures the weather.
func BenchmarkProfileWalk(b *testing.B) {
	dir := os.Getenv("MESHBENCH_TERRAIN_DIR")
	if dir == "" {
		b.Skip("MESHBENCH_TERRAIN_DIR not set")
	}
	st, err := NewTileStore(dir)
	if err != nil {
		b.Fatal(err)
	}
	// Scotland's extent, the study area the slow warm was measured on.
	const latLo, latHi, lonLo, lonHi = 54.8, 58.6, -7.5, -1.8
	rng := rand.New(rand.NewSource(4417))
	type pair struct{ aLat, aLon, bLat, bLon float64 }
	pairs := make([]pair, 512)
	for i := range pairs {
		pairs[i] = pair{
			latLo + rng.Float64()*(latHi-latLo), lonLo + rng.Float64()*(lonHi-lonLo),
			latLo + rng.Float64()*(latHi-latLo), lonLo + rng.Float64()*(lonHi-lonLo),
		}
	}
	b.ResetTimer()
	samples, misses := 0, 0
	for i := 0; i < b.N; i++ {
		p := pairs[i%len(pairs)]
		const steps = 256
		for k := 0; k <= steps; k++ {
			f := float64(k) / float64(steps)
			_, ok := st.ElevationM(p.aLat+(p.bLat-p.aLat)*f, p.aLon+(p.bLon-p.aLon)*f)
			samples++
			if !ok {
				misses++
			}
		}
	}
	b.ReportMetric(float64(samples)/b.Elapsed().Seconds(), "samples/s")
	b.ReportMetric(float64(misses), "misses")
}
