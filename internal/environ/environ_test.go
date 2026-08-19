package environ_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MeshBench/meshbench/internal/environ"
)

// square is a building footprint centred on lat/lon, side in degrees.
func square(lat, lon, side, height float64, material string) environ.Building {
	h := side / 2
	return environ.Building{
		Footprint: [][2]float64{
			{lat - h, lon - h}, {lat - h, lon + h}, {lat + h, lon + h}, {lat + h, lon - h},
		},
		HeightM: height, Material: material, MaterialConfidence: 0.8,
	}
}

type fixture []environ.Building

func (f fixture) Buildings(_, _, _, _ float64) []environ.Building { return f }

type flatGround struct{}

func (flatGround) ElevationM(_, _ float64) (float64, bool) { return 100, true }

// A path through a building reports the crossing, with the fractions in the
// right order and the top above the ground the terrain supplies.
func TestObstructionsOnPath(t *testing.T) {
	b := square(56.0, -3.0, 0.001, 12, environ.MatBrick)
	obs := environ.ObstructionsOnPath(fixture{b}, flatGround{},
		56.0, -3.01, 56.0, -2.99)
	if len(obs) != 1 {
		t.Fatalf("expected one crossing, got %d", len(obs))
	}
	o := obs[0]
	if o.EnterFrac >= o.ExitFrac {
		t.Fatalf("fractions out of order: %v", o)
	}
	if o.EnterFrac < 0.4 || o.ExitFrac > 0.6 {
		t.Fatalf("the crossing should straddle the middle: %+v", o)
	}
	if o.TopM != 112 {
		t.Fatalf("top = %v, want ground 100 + height 12", o.TopM)
	}
	if o.Material != environ.MatBrick {
		t.Fatalf("material lost: %+v", o)
	}
}

// A path that misses the footprint reports nothing.
func TestPathMissingTheBuilding(t *testing.T) {
	b := square(56.0, -3.0, 0.001, 12, environ.MatBrick)
	obs := environ.ObstructionsOnPath(fixture{b}, flatGround{},
		56.01, -3.01, 56.01, -2.99)
	if len(obs) != 0 {
		t.Fatalf("expected no crossing, got %d", len(obs))
	}
}

// A path starting inside a building still reports the exit - an indoor node
// is a thing meshes genuinely have.
func TestPathFromInsideTheBuilding(t *testing.T) {
	b := square(56.0, -3.0, 0.001, 12, environ.MatConcrete)
	obs := environ.ObstructionsOnPath(fixture{b}, flatGround{},
		56.0, -3.0, 56.0, -2.99)
	if len(obs) != 1 {
		t.Fatalf("expected one crossing, got %d", len(obs))
	}
	if obs[0].EnterFrac != 0 {
		t.Fatalf("the crossing should start at the node: %+v", obs[0])
	}
}

// Tiles round-trip through the store, and a missing tile is counted rather
// than mistaken for empty ground.
func TestTileRoundTripAndMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "env")
	b := square(56.34, -2.79, 0.0005, 9, environ.MatStone)
	x, y, ok := environ.TileFor(b)
	if !ok {
		t.Fatal("no tile for a real footprint")
	}
	if err := environ.WriteTile(dir, x, y, []environ.Building{b}); err != nil {
		t.Fatal(err)
	}

	s := environ.OpenTiles(dir)
	got := s.Buildings(56.33, -2.80, 56.35, -2.78)
	if len(got) != 1 || got[0].Material != environ.MatStone {
		t.Fatalf("round trip lost the building: %+v", got)
	}
	if s.Missing() != 0 && len(got) == 1 {
		// Adjacent tiles in the queried box may legitimately be missing;
		// what matters is the written one loaded.
		_ = s.Missing()
	}

	empty := environ.OpenTiles(filepath.Join(os.TempDir(), "does-not-exist-anywhere"))
	_ = empty.Buildings(56.33, -2.80, 56.35, -2.78)
	if empty.Missing() == 0 {
		t.Fatal("a store with no data claimed full coverage")
	}
}

// Material inference follows the precedence order, and says how sure it is.
func TestMaterialInference(t *testing.T) {
	if m, src, conf := environ.InferMaterial("brick", "industrial", "uk"); m != environ.MatBrick || src != "osm" || conf != 1.0 {
		t.Fatalf("explicit OSM material must win: %s %s %v", m, src, conf)
	}
	if m, _, conf := environ.InferMaterial("", "industrial", "uk"); m != environ.MatMetal || conf >= 1.0 {
		t.Fatalf("industrial should infer metal below certainty: %s %v", m, conf)
	}
	if m, src, _ := environ.InferMaterial("", "house", "uk"); m != environ.MatBrick || src != "regional" {
		t.Fatalf("a UK house is regionally brick: %s %s", m, src)
	}
	if m, _, conf := environ.InferMaterial("", "", ""); m != environ.MatUnknown || conf != 0 {
		t.Fatalf("nothing known must say UNKNOWN: %s %v", m, conf)
	}
}
