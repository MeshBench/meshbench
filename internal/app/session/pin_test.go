package session

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// Setting a firmware version has to reach the engine, because the engine's
// copy of the spec is what starts a process.
//
// This is the bug 0.0.3 shipped: the library reported 273 nodes pinned to
// v1.17.1, the panel drew v1.17.1, and starting firmware asked for the
// v1.17.0 the network had been opened with - so the failure read as "the
// version I chose is not published" and sent everybody to the catalogue.
// Two lists a person can see agreed with each other and disagreed with the
// one that runs.
func TestPinningFirmwareReachesTheEngine(t *testing.T) {
	store := state.New(10)
	sim := &Sim{}
	Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)

	if _, err := store.Do(ctx, "project.open", "../../../fixtures/fixture-fife-strict.json"); err != nil {
		t.Skip("no fixture:", err)
	}

	const want = "repeater-v9.9.9-test"
	if _, err := store.Do(ctx, "firmware.set", map[string]any{
		"role": "simple_repeater", "version": want,
	}); err != nil {
		t.Fatal(err)
	}

	// What the panel would draw.
	pinned := 0
	for _, n := range sim.nodes {
		if n.Firmware.Version == want {
			pinned++
		}
	}
	if pinned == 0 {
		t.Fatal("nothing was pinned in the scenario; the fixture may have changed")
	}

	// And what will actually start.
	inEngine := 0
	for _, n := range sim.eng.Nodes() {
		if n.Spec.Firmware.Version == want {
			inEngine++
		}
	}
	if inEngine != pinned {
		t.Errorf("the scenario says %d nodes run %s and the engine says %d - "+
			"a pin that does not reach the engine starts the wrong build",
			pinned, want, inEngine)
	}
}

// The same for one node at a time, which is the path the node window uses.
func TestPinningOneNodeReachesTheEngine(t *testing.T) {
	store := state.New(10)
	sim := &Sim{}
	Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)

	if _, err := store.Do(ctx, "project.open", "../../../fixtures/fixture-fife-strict.json"); err != nil {
		t.Skip("no fixture:", err)
	}
	if len(sim.nodes) == 0 {
		t.Skip("no nodes")
	}
	name := sim.nodes[0].Name
	const want = "repeater-v8.8.8-test"
	if err := sim.setFirmware(name, want); err != nil {
		t.Fatal(err)
	}
	for _, n := range sim.eng.Nodes() {
		if n.Spec.Name == name {
			if n.Spec.Firmware.Version != want {
				t.Fatalf("%s: engine has %q, the scenario has %q",
					name, n.Spec.Firmware.Version, want)
			}
			return
		}
	}
	t.Fatalf("%s is not in the engine at all", name)
}
