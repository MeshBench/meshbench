package terrain_test

import (
	"testing"

	"github.com/A13xB0/meshcoresim/internal/terrain"
)

// One hill is one edge, not the forty samples that describe it. A decomposition
// that reports every rising sample is a list, not an explanation.
func TestEdgesReportsOneEdgePerHill(t *testing.T) {
	var profile []terrain.Point
	for i := 0; i <= 200; i++ {
		d := float64(i) * 100 // 20 km
		h := 100.0
		// A single ridge at 8 km, 400 m above the surrounding ground.
		if i > 60 && i < 100 {
			x := float64(i-80) / 20
			h += 400 * (1 - x*x)
		}
		profile = append(profile, terrain.Point{DistM: d, HeightM: h})
	}
	edges := terrain.Edges(profile, 10, 10, 869.6)
	if len(edges) != 1 {
		t.Fatalf("one ridge produced %d edges", len(edges))
	}
	e := edges[0]
	if e.DistM < 7000 || e.DistM > 9000 {
		t.Errorf("the edge is at %.0f m; the ridge is at 8000", e.DistM)
	}
	if e.LossDB < 5 {
		t.Errorf("a 400 m ridge across the path cost only %.1f dB", e.LossDB)
	}
	// The clearance must say the terrain stands above the sight line, which is
	// the fact that makes the loss believable.
	if e.ClearanceM >= 0 {
		t.Errorf("clearance %.0f m says the ridge is below the line of sight", e.ClearanceM)
	}
}

// A clear path has nothing to explain, and inventing edges for it would put
// obstructions on a profile anyone can see is flat.
func TestEdgesFindsNothingOnAClearPath(t *testing.T) {
	var profile []terrain.Point
	for i := 0; i <= 100; i++ {
		profile = append(profile, terrain.Point{DistM: float64(i) * 50, HeightM: 100})
	}
	if edges := terrain.Edges(profile, 30, 30, 869.6); len(edges) != 0 {
		t.Errorf("flat ground produced %d edges", len(edges))
	}
}
