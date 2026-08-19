package session

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/environ"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

type flatGround struct{}

func (flatGround) ElevationM(_, _ float64) (float64, bool) { return 10, true }

type slabProvider struct{ b environ.Building }

func (s slabProvider) Buildings(_, _, _, _ float64) []environ.Building {
	return []environ.Building{s.b}
}

// The GPU warm's building term is the engine's building term: the same
// arithmetic through the indexed twin, fed the same endpoints. A slab
// across the path must price the same decibels as the direct environ call,
// and a pair already dead on terrain alone must skip the walk entirely.
func TestGPUWarmPricesBuildingsLikeTheEngine(t *testing.T) {
	// A 30 m slab squarely across the midpoint of a 2 km hop.
	env := slabProvider{b: environ.Building{
		HeightM: 30,
		Footprint: [][2]float64{
			{56.0099, -3.19}, {56.0101, -3.19}, {56.0101, -3.17}, {56.0099, -3.17},
		},
	}}
	na := scenario.Node{HeightAGLm: 8}
	na.Position.Lat, na.Position.Lon = 56.0, -3.18
	nb := scenario.Node{HeightAGLm: 8}
	nb.Position.Lat, nb.Position.Lon = 56.02, -3.18
	heights := []float32{10, 10, 10, 10, 10}
	const distM, freq = 2226.0, 869.525

	ix := pathIndexOver(env, flatGround{}, []scenario.Node{na, nb})
	got := pairBuildingLossDB(ix, na, nb, heights, 8, 8, distM, freq, 120)
	want := environ.PathBuildingLossDB(env, flatGround{},
		na.Position.Lat, na.Position.Lon, 18,
		nb.Position.Lat, nb.Position.Lon, 18, distM, freq)
	if got != want {
		t.Fatalf("warm prices %f dB, the engine's call prices %f dB", got, want)
	}
	if got <= 0 {
		t.Fatalf("a 30 m slab across an 8 m path priced %f dB; the fixture is broken", got)
	}
	if dead := pairBuildingLossDB(ix, na, nb, heights, 8, 8,
		distM, freq, 200); dead != 0 {
		t.Fatalf("a pair dead on terrain alone paid %f dB for the index walk", dead)
	}
}
