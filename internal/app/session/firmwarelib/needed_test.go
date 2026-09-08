package firmwarelib

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// firmware.needed answers by role, the way the engine resolves. It used to
// take a pin on its version alone, so a companion pinned to a repeater build
// the cache held was not needed, and a script asking what a run was short of
// was told nothing while six nodes could never start.
func TestANodePinnedToAnotherRolesBuildIsStillNeeded(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	dir := filepath.Join(cache, "meshbench", "firmware", "native", "repeater-v1.17.1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, firmware.NativeBinaryName("simple_repeater"))
	if err := os.WriteFile(bin, []byte("#!/bin/true\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	nodes := []scenario.Node{
		{Name: "hill", Kind: scenario.SimpleRepeater, Firmware: scenario.FirmwareRef{Version: "repeater-v1.17.1"}},
		{Name: "phone", Kind: scenario.Companion, Firmware: scenario.FirmwareRef{Version: "repeater-v1.17.1"}},
	}
	store := state.New(10)
	sim := &session.Sim{}
	sim.BuildSeeded(nodes, 869.618, 1)
	registerFirmwareLibrary(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)

	out, err := store.Do(ctx, "firmware.needed", nil)
	if err != nil {
		t.Fatal(err)
	}
	roles := out.(map[string]any)["roles"].([]any)
	if len(roles) != 1 {
		t.Fatalf("the companion alone needs a build, got %v", roles)
	}
	row := roles[0].(map[string]any)
	if row["role"] != "companion_radio" || row["nodes"] != 1 {
		t.Errorf("want companion_radio x1, got %v", row)
	}
}
