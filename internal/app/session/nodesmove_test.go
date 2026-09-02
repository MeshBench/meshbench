package session

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// nodes.move takes the parameter its siblings take.
//
// Every other verb that acts on a node it did not create calls it "node":
// nodes.delete, nodes.regions, node.wipe, node.truerf, sim.inject. This one
// asked for "name" alone, so a script that had learnt the surface everywhere
// else was refused here and nowhere else - and the refusal named a parameter
// the caller had no reason to expect, which is the sort of message somebody
// reads three times before believing.
func TestMoveTakesTheNodeParameterItsSiblingsTake(t *testing.T) {
	st, _ := aNetwork(t)
	ctx := t.Context()

	got, err := st.Do(ctx, "nodes.move", map[string]any{
		"node": "West Lomond", "lat": 56.1, "lon": -3.1})
	if err != nil {
		t.Fatalf(`nodes.move refused {"node": ...}: %v`, err)
	}
	// Answered, and answered about the node that was asked for.
	if m := mapOf(t, got); m["node"] != "West Lomond" {
		t.Errorf("the reply names %v, not the node that moved", m["node"])
	}
	// And it moved: a verb that answers success without doing the thing is the
	// shape this whole change is about, so the position is read back.
	atNode(t, st, "West Lomond", 56.1, -3.1)
}

// The older spelling keeps working, because it is in saved scripts and in
// every version of the documentation published so far.
func TestMoveStillTakesTheOlderNameParameter(t *testing.T) {
	st, _ := aNetwork(t)
	if _, err := st.Do(t.Context(), "nodes.move", map[string]any{
		"name": "Dunfermline", "lat": 56.0, "lon": -3.5}); err != nil {
		t.Fatalf(`nodes.move refused {"name": ...}: %v`, err)
	}
	atNode(t, st, "Dunfermline", 56.0, -3.5)
}

// A node this network has not got is still refused under either spelling, so
// the new parameter did not open a route past the name check.
func TestMoveRefusesAnUnknownNodeUnderEitherSpelling(t *testing.T) {
	st, _ := aNetwork(t)
	for _, key := range []string{"node", "name"} {
		msg := refuses(t, st, "nodes.move", map[string]any{
			key: "West Lomand", "lat": 56.2, "lon": -3.3})
		mentions(t, msg, "West Lomand")
	}
}

// atNode fails unless the named node is where it was put.
func atNode(t *testing.T, st *state.Store, name string, lat, lon float64) {
	t.Helper()
	for _, n := range st.Snapshot().Nodes {
		if n.Name != name {
			continue
		}
		if n.Lat != lat || n.Lon != lon {
			t.Fatalf("%s is at %v,%v; it was moved to %v,%v", name, n.Lat, n.Lon, lat, lon)
		}
		return
	}
	t.Fatalf("there is no node called %q", name)
}
