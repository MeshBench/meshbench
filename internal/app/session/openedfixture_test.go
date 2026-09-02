package session

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// openedFixture is a session with a shipped network in it, the way project.open
// leaves one.
//
// Through the verb rather than through LoadFixture, because the fault this file
// is about was invisible to every test that called the builder directly: the
// builder that fills the field and the builder the open path uses were two
// different functions, and each had tests of its own that passed.
func openedFixture(t *testing.T) (*state.Store, context.Context) {
	t.Helper()
	store := state.New(10)
	Register(store, &Sim{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go store.Run(ctx)
	if _, err := store.Do(ctx, "project.open",
		"../../../fixtures/fixture-fem-e22.json"); err != nil {
		t.Skip("no fixture:", err)
	}
	return store, ctx
}

// Does a node opened from a fixture know what it is?
//
// It did not. nodes.list publishes Hardware as `board`, describing it as what
// the node is, and only the import path ever set it: everything that arrived
// through project.open or session.restore reported `"board": ""` for every node
// in the network, which is every node anybody has after opening a saved one.
func TestAnOpenedFixtureKnowsWhatItsNodesAre(t *testing.T) {
	store, ctx := openedFixture(t)
	reply, err := store.Do(ctx, "nodes.list", nil)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := reply.(map[string]any)["nodes"].([]map[string]any)
	if len(rows) == 0 {
		t.Fatal("nodes.list answered with no rows")
	}
	for _, row := range rows {
		if row["board"] == "" {
			t.Errorf("%v reports no board after the fixture opened; the fixture "+
				"names one, and only the import path was setting it", row["name"])
		}
	}
}

// Does a board with a card slot say so before anybody touches its firmware?
//
// The five card fields were written by publishCards, which runs on a firmware
// change and on nothing else, and by the import builder. Opening a fixture ran
// neither, so a board with a slot reported no slot until something unrelated
// happened to it.
func TestAnOpenedFixtureKnowsAboutCardSlots(t *testing.T) {
	store, _ := openedFixture(t)
	nodes := store.Snapshot().Nodes
	if len(nodes) == 0 {
		t.Fatal("the fixture opened with no nodes")
	}
	var withBoard int
	for _, n := range nodes {
		if n.Hardware == "" {
			continue
		}
		withBoard++
		if n.CardSlot != boardHasCardSlot(n.Hardware) {
			t.Errorf("%s: card slot reads %v, and its board says %v",
				n.Name, n.CardSlot, boardHasCardSlot(n.Hardware))
		}
	}
	if withBoard == 0 {
		t.Fatal("no node in the fixture carries a board, so this proves nothing")
	}
}

// holedArea is a GeoJSON polygon with a loch in the middle of it.
func holedArea() string {
	outer := "[[-3.6,56.0],[-3.2,56.0],[-3.2,56.4],[-3.6,56.4],[-3.6,56.0]]"
	inner := "[[-3.45,56.15],[-3.35,56.15],[-3.35,56.25],[-3.45,56.25],[-3.45,56.15]]"
	return `{"type":"FeatureCollection","features":[{"type":"Feature",` +
		`"properties":{"name":"Kinross"},` +
		`"geometry":{"type":"Polygon","coordinates":[` + outer + `,` + inner + `]}}]}`
}

// Does a loch stay a loch on the way to the map?
//
// The map and the boundary panel both draw state.Area.Holes, and one of the
// three places that build an area filled it. The GeoJSON reader parses interior
// rings and the scenario carries them, and then both boundary constructors
// walked the outer rings alone - so an area loaded through boundary.load
// covered the water inside it while the same geometry opened from a fixture did
// not, and nothing said which of the two was being drawn.
func TestALoadedBoundaryKeepsItsHoles(t *testing.T) {
	store := state.New(10)
	Register(store, &Sim{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)
	if _, err := store.Do(ctx, "boundary.load",
		map[string]any{"geojson": holedArea()}); err != nil {
		t.Fatal(err)
	}
	areas := store.Snapshot().Areas
	if len(areas) != 1 {
		t.Fatalf("%d study areas after the load, want 1", len(areas))
	}
	if len(areas[0].Rings) != 1 {
		t.Errorf("%d rings, want 1", len(areas[0].Rings))
	}
	if len(areas[0].Holes) != 1 {
		t.Errorf("%d holes, want the one the document holds; the map draws them "+
			"and the reader parsed them, so an empty list is a claim about the "+
			"place rather than about the file", len(areas[0].Holes))
	}
}
