package engine_test

import (
	"context"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// How long does the link matrix actually take? The answer decides whether a
// GPU path is worth building, so it is measured rather than assumed.
func TestLiveWarmSpeed(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}
	rng := rand.New(rand.NewSource(4417))
	e := engine.New(flat{}, engine.Config{
		FreqMHz: 869.525, SF: 10, BandwidthHz: 250e3, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417,
	})
	defer func() { _ = e.Close() }()

	mast := antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6}, Polarisation: "vertical"}
	for i := 0; i < 400; i++ {
		e.Add(scenario.Node{
			Name: "n", Kind: scenario.SimpleRepeater,
			Position: scenario.LatLon{
				Lat: 56.30 + rng.Float64()*0.55,
				Lon: -4.20 + rng.Float64()*0.85,
			},
			HeightAGLm: 10, Antenna: mast, TxPowerDBm: 22, NoiseFigureDB: 6,
			Radio: scenario.RadioConfig{CentreHz: 869.525e6, BandwidthHz: 250e3, SpreadFactor: 10, CodingRate: 1},
		}, nil)
	}

	start := time.Now()
	e.WarmLinks(context.Background(), nil)
	t.Logf("400 nodes, 79800 pairs, flat terrain: warmed in %v", time.Since(start))
}

// The same measurement over the real DEM, which is where the cost actually
// lives: every profile is ~256 bilinear tile reads.
func TestLiveWarmSpeedRealDEM(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}
	cache, _ := os.UserCacheDir()
	store, err := terrain.NewTileStore(cache + "/meshbench/terrain")
	if err != nil {
		t.Fatal(err)
	}
	store.Offline = true // measure the cache, not the network

	rng := rand.New(rand.NewSource(4417))
	e := engine.New(store, engine.Config{
		FreqMHz: 869.525, SF: 10, BandwidthHz: 250e3, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417,
	})
	defer func() { _ = e.Close() }()

	mast := antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6}, Polarisation: "vertical"}
	for i := 0; i < 400; i++ {
		e.Add(scenario.Node{
			Name: "n", Kind: scenario.SimpleRepeater,
			// The Perthshire box the demo network lives in, where tiles are
			// already cached on the development machine.
			Position: scenario.LatLon{
				Lat: 56.45 + rng.Float64()*0.30,
				Lon: -3.95 + rng.Float64()*0.40,
			},
			HeightAGLm: 10, Antenna: mast, TxPowerDBm: 22, NoiseFigureDB: 6,
			Radio: scenario.RadioConfig{CentreHz: 869.525e6, BandwidthHz: 250e3, SpreadFactor: 10, CodingRate: 1},
		}, nil)
	}

	start := time.Now()
	e.WarmLinks(context.Background(), nil)
	elapsed := time.Since(start)

	// How much of the matrix actually resolved, so a cold tile cache reads as
	// "no data measured" rather than as a fast result.
	resolved := 0
	for a := 0; a < 400; a++ {
		for b := a + 1; b < 400; b++ {
			if _, ok := e.PathLossForTest(a, b); ok {
				resolved++
			}
		}
	}
	t.Logf("400 nodes, 79800 pairs, real DEM: warmed in %v, %d pairs had terrain", elapsed, resolved)
}
