package session

import (
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/rf/terrain"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// The warm prefetches the ground under its lines, never the box around its
// fleet: the rectangle around a coastal country is mostly open sea no profile
// crosses, and prefetching it fetched twenty-eight thousand tiles of Atlantic
// while the operator watched the warm sit on zero percent.
func TestProfileTilesIsLinesNotABox(t *testing.T) {
	// Two clusters far apart: the lines between them are a narrow diagonal
	// band, and the box around them is everything.
	var nodes []scenario.Node
	for i := 0; i < 4; i++ {
		n := scenario.Node{}
		n.Position.Lat, n.Position.Lon = 55.0+0.01*float64(i), -6.0
		nodes = append(nodes, n)
		m := scenario.Node{}
		m.Position.Lat, m.Position.Lon = 58.0, -2.0+0.01*float64(i)
		nodes = append(nodes, m)
	}
	tiles := profileTiles(nodes, terrain.DefaultZoom)
	boxCount, _, _, _, _ := terrain.TilesForBounds(55.0, 58.0, -6.0, -2.0, terrain.DefaultZoom)
	if len(tiles) == 0 {
		t.Fatal("no tiles at all")
	}
	if len(tiles) >= boxCount/2 {
		t.Fatalf("the line set holds %d tiles against the box's %d; that is a "+
			"box wearing a different name", len(tiles), boxCount)
	}
}

// A country-sized fleet's tile set must be computable in moments: it runs at
// the head of every cold warm, and a pre-pass that costs minutes is the very
// stall it exists to prevent.
func TestProfileTilesIsQuickAtCountryScale(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	var nodes []scenario.Node
	for i := 0; i < 450; i++ {
		n := scenario.Node{}
		n.Position.Lat = 51.5 + 7.0*float64(i%21)/21
		n.Position.Lon = -10.0 + 11.0*float64(i%23)/23
		nodes = append(nodes, n)
	}
	start := time.Now()
	tiles := profileTiles(nodes, terrain.DefaultZoom)
	took := time.Since(start)
	if took > 15*time.Second {
		t.Fatalf("computing %d tiles for 450 nodes took %v; the pre-pass has "+
			"become the stall", len(tiles), took)
	}
	t.Logf("450 nodes: %d tiles in %v", len(tiles), took)
}
