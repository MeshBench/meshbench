package session

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/scenario"
)

func nodeAt(name string, kind scenario.Kind, lat, lon float64) scenario.Node {
	n := scenario.Node{Name: name, Kind: kind}
	n.Position.Lat, n.Position.Lon = lat, lon
	return n
}

// The shared box must hold every node with room past the outermost mast, and
// its pixel grid must follow the ground's aspect, not the degrees'.
func TestMapBoxCoversEveryNodeWithMargin(t *testing.T) {
	nodes := []scenario.Node{
		nodeAt("a", scenario.SimpleRepeater, 56.0, -4.0),
		nodeAt("b", scenario.SimpleRepeater, 56.2, -3.0),
	}
	south, north, west, east, w, h, err := mapBox(nodes)
	if err != nil {
		t.Fatal(err)
	}
	if south >= 56.0 || north <= 56.2 || west >= -4.0 || east <= -3.0 {
		t.Fatalf("box [%f %f %f %f] does not clear the nodes", south, north, west, east)
	}
	// 0.2 deg of latitude is ~22 km; 1 deg of longitude at 56N is ~62 km.
	// East-west is the long way, so the grid's long edge must be east-west.
	if w <= h {
		t.Fatalf("grid %dx%d ignores the ground's aspect", w, h)
	}
	if w != mapGridMax {
		t.Fatalf("long edge is %d, want %d", w, mapGridMax)
	}
}

func TestMapBoxRefusesEmptiness(t *testing.T) {
	if _, _, _, _, _, _, err := mapBox(nil); err == nil {
		t.Fatal("an empty network produced a box")
	}
}

// Companions and observers must not paint "coverage": one is somebody's
// pocket and the other never transmits.
func TestInfrastructureExcludesPocketsAndObservers(t *testing.T) {
	nodes := []scenario.Node{
		nodeAt("r", scenario.SimpleRepeater, 56, -3),
		nodeAt("a", scenario.AdvancedRepeater, 56, -3),
		nodeAt("s", scenario.RoomServer, 56, -3),
		nodeAt("c", scenario.Companion, 56, -3),
		nodeAt("o", scenario.SDRObserver, 56, -3),
	}
	got := infrastructure(nodes)
	if len(got) != 3 {
		t.Fatalf("kept %d nodes, want the 3 infrastructure kinds", len(got))
	}
	for _, n := range got {
		if n.Kind == scenario.Companion || n.Kind == scenario.SDRObserver {
			t.Fatalf("%s (%s) is not infrastructure", n.Name, n.Kind)
		}
	}
}
