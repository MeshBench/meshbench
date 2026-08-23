// Finding a node by name, against a real workbench.
package meshbench

import (
	"errors"
	"testing"
)

// Finding a node whose name you cannot type, which is the case the ScotMesh
// example is built on: real imported names carry emoji either side and
// sometimes an accent, so every script that wanted one node was reduced to
// fetching all of them and matching by hand.
func TestFindingANodeWhoseNameYouCannotType(t *testing.T) {
	wb, ctx := headless(t)
	if err := wb.Project().New(ctx, ""); err != nil {
		t.Fatal(err)
	}
	const lomond = "\U0001F3D4️ West Lomond \U0001F4E1"
	for _, p := range []Placement{
		{Name: lomond, Lat: 56.24, Lon: -3.29},
		{Name: "West Lomond Relay Two", Lat: 56.25, Lon: -3.28},
		{Name: "Beinn Àrd ⛰", Lat: 56.30, Lon: -3.40},
		{Name: "\U0001F4FB Dunfermline Repeater", Lat: 56.07, Lon: -3.46},
	} {
		if _, err := wb.Nodes().Place(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	hits, err := wb.Nodes().Search(ctx, "west lomond", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) < 2 {
		t.Fatalf("the emoji name was not found: %+v", hits)
	}
	// The exact name beats the one that merely starts the same way. A caller
	// taking the top result is taking this.
	if hits[0].Name != lomond || hits[0].Score <= hits[1].Score {
		t.Errorf("ranking put %q (%.2f) above %q (%.2f)",
			hits[1].Name, hits[1].Score, hits[0].Name, hits[0].Score)
	}

	// Accents fold, so the Gaelic name is reachable from an ASCII keyboard.
	n, err := wb.Nodes().Find(ctx, "beinn ard")
	if err != nil {
		t.Fatal(err)
	}
	if n.Name() != "Beinn Àrd ⛰" {
		t.Errorf("accent-folded search found %q", n.Name())
	}

	// Find refuses rather than handing back the nearest thing, and says what
	// it did find - the difference between a typo and an absence.
	if _, err := wb.Nodes().Find(ctx, "Ben Nevis"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a query nothing matches gave %v, want a not-found refusal", err)
	}
}
