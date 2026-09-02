package session

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A pin aimed at one role must reach that role and nothing else, on both sides.
//
// The count was right and the set was not: the engine's side of the verb
// honoured the role and the side that decides what is drawn ignored it, so
// choosing a repeater build redrew every companion and room server in the mesh
// as running it too. Asserting on how many changed is exactly what missed it,
// so this asserts on which.
func TestSettingFirmwareByRoleLeavesTheOtherRolesAlone(t *testing.T) {
	nodes := []scenario.Node{
		{Name: "hill", Kind: scenario.SimpleRepeater,
			Firmware: scenario.FirmwareRef{Version: "before"}},
		{Name: "phone", Kind: scenario.Companion,
			Firmware: scenario.FirmwareRef{Version: "before"}},
		{Name: "posts", Kind: scenario.RoomServer,
			Firmware: scenario.FirmwareRef{Version: "before"}},
	}
	store := state.New(10)
	sim := &Sim{nodes: nodes}
	registerFirmwareNodes(store, sim)
	store.Handle("test.nodes", func(w *state.World, _ any) (any, error) {
		for _, n := range sim.nodes {
			w.Nodes = append(w.Nodes, state.Node{
				Name: n.Name, Kind: string(n.Kind), Firmware: n.Firmware.Version,
			})
		}
		return nil, nil
	})
	var world *state.World
	store.Handle("test.world", func(w *state.World, p any) (any, error) {
		*(p.(**state.World)) = w
		return nil, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)
	if _, err := store.Do(ctx, "test.nodes", nil); err != nil {
		t.Fatal(err)
	}

	const want = "repeater-v9.9.9-test"
	if _, err := store.Do(ctx, "firmware.set", map[string]any{
		"role": "simple_repeater", "version": want,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Do(ctx, "test.world", &world); err != nil {
		t.Fatal(err)
	}

	drawn := map[string]string{}
	for _, n := range world.Nodes {
		drawn[n.Name] = n.Firmware
	}
	for _, n := range sim.nodes {
		expect := "before"
		if n.Name == "hill" {
			expect = want
		}
		if n.Firmware.Version != expect {
			t.Errorf("%s runs %q, want %q", n.Name, n.Firmware.Version, expect)
		}
		if drawn[n.Name] != expect {
			t.Errorf("%s is drawn as %q, want %q - the view was written by a "+
				"second walk that did not honour the role",
				n.Name, drawn[n.Name], expect)
		}
	}
}

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
		if n.Spec().Firmware.Version == want {
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
	if err := sim.setFirmware(name, Build{Version: want}); err != nil {
		t.Fatal(err)
	}
	for _, n := range sim.eng.Nodes() {
		if n.Spec().Name == name {
			if n.Spec().Firmware.Version != want {
				t.Fatalf("%s: engine has %q, the scenario has %q",
					name, n.Spec().Firmware.Version, want)
			}
			return
		}
	}
	t.Fatalf("%s is not in the engine at all", name)
}

// The board travels with the version, and only when it is passed.
//
// firmware.set read version, node and role and nothing else, so a board handed
// to the bulk verb was dropped: the node kept its hardware, lost its image, and
// the start then failed asking for a native build of a version that has none.
// That message is correct for what the engine was asked to do and useless for
// what the operator meant, and it sends the reader off to build MeshCore from
// source over a field they had passed.
func TestSettingFirmwareCarriesTheBoardAndOnlyWhenAsked(t *testing.T) {
	nodes := []scenario.Node{
		{Name: "emu", Kind: scenario.SimpleRepeater,
			Firmware: scenario.FirmwareRef{Version: "before", Board: "Heltec_v3"}},
		{Name: "host", Kind: scenario.SimpleRepeater,
			Firmware: scenario.FirmwareRef{Version: "before"}},
	}
	store := state.New(10)
	sim := &Sim{nodes: nodes}
	registerFirmwareNodes(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)

	board := func(name string) string {
		for _, n := range sim.nodes {
			if n.Name == name {
				return n.Firmware.Board
			}
		}
		t.Fatalf("no node %q", name)
		return ""
	}

	// No board passed: every board stays as it was. The single-node verb reads
	// an absent board as "a host build" and clears it, and doing that here
	// would convert a whole mesh of emulated nodes to native on a call that
	// only named a version.
	if _, err := store.Do(ctx, "firmware.set", map[string]any{"version": "v2"}); err != nil {
		t.Fatal(err)
	}
	if got := board("emu"); got != "Heltec_v3" {
		t.Errorf("a call naming no board cleared one: emu is on %q", got)
	}

	// Passed: it is written, which is what makes "pin every repeater to this
	// image" a thing the bulk verb can do at all.
	got, err := store.Do(ctx, "firmware.set", map[string]any{
		"version": "v3", "board": "RAK_4631",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"emu", "host"} {
		if b := board(name); b != "RAK_4631" {
			t.Errorf("%s is on %q, want RAK_4631", name, b)
		}
	}
	if m, _ := got.(map[string]any); m["board"] != "RAK_4631" {
		t.Errorf("the reply does not echo the board it set: %v", got)
	}

	// And an explicit empty string is how a node goes back to a host build,
	// which is the only reason presence has to be read rather than emptiness.
	if _, err := store.Do(ctx, "firmware.set", map[string]any{
		"version": "v4", "board": "",
	}); err != nil {
		t.Fatal(err)
	}
	if got := board("emu"); got != "" {
		t.Errorf("an explicit empty board left emu on %q", got)
	}
}
