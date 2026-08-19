package engine

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// countingTerrain counts DEM lookups - the currency the profile cache saves.
type countingTerrain struct{ lookups int }

func (c *countingTerrain) ElevationM(_, _ float64) (float64, bool) {
	c.lookups++
	return 100, true
}

func profNode(name string, lon float64) scenario.Node {
	return scenario.Node{
		Name: name, Kind: scenario.SimpleRepeater,
		Position: scenario.LatLon{Lat: 56.5, Lon: lon}, HeightAGLm: 10,
		Radio:      scenario.RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62500, SpreadFactor: 8, CodingRate: 4},
		TxPowerDBm: 22, NoiseFigureDB: 6,
		Antenna: antenna.Mounted{Pattern: antenna.Dipole{}, Polarisation: "vertical"},
	}
}

// A radio report must cost arithmetic, not a DEM walk: the loss is
// invalidated, the profile survives, and the recomputed loss is identical
// because it is built from the identical ground. This is the stall where a
// busy network stuttered to a stop - every transmission's FEM report
// re-walked the terrain under every active pair.
func TestRadioReportDoesNotRewalkTheTerrain(t *testing.T) {
	terr := &countingTerrain{}
	e := New(terr, Config{StepMs: 10, Seed: 5})
	e.Add(profNode("a", -3.9), nil)
	e.Add(profNode("b", -3.5), nil)

	l1, ok := e.pathLoss(0, 1)
	if !ok {
		t.Fatal("no path loss on flat ground")
	}
	walked := terr.lookups
	if walked == 0 {
		t.Fatal("the first loss cost no DEM lookups; the test is not testing")
	}

	// The radio-report invalidation path: node 0's loss entries drop.
	e.mu.Lock()
	for k := range e.linkCache {
		delete(e.linkCache, k)
	}
	e.mu.Unlock()

	l2, ok := e.pathLoss(0, 1)
	if !ok {
		t.Fatal("loss vanished after invalidation")
	}
	if terr.lookups != walked {
		t.Fatalf("recomputing the loss walked the DEM again (%d -> %d lookups); "+
			"the profile cache is not caching", walked, terr.lookups)
	}
	if l1 != l2 {
		t.Fatalf("same ground, different loss: %f then %f", l1, l2)
	}
}
