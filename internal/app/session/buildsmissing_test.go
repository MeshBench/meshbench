package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// The readiness gate has to ask the question the engine asks.
//
// firmware.Resolve tries FindNative before it looks in the cache, so an
// override supplies a node's build whatever version it is pinned to. Reading
// the cache alone made this gate stricter than the thing it guards: it refused
// runs that would have started, and told the operator to pin a build in the
// Firmware panel, which is the one thing that could not have helped.
func TestAnOverrideSatisfiesAPinTheCacheHasNever(t *testing.T) {
	// A version no cache will hold, so only the override can answer for it.
	const pinned = "repeater-v0.0.0-not-a-release"

	dir := t.TempDir()
	bin := filepath.Join(dir, firmware.NativeBinaryName("simple_repeater"))
	if err := os.WriteFile(bin, []byte("#!/bin/true\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	node := repeaterNode("Solo")
	node.Firmware = scenario.FirmwareRef{Version: pinned}
	s := &Sim{nodes: []scenario.Node{node}}

	t.Setenv(firmware.EnvNativeBinary, "")
	if got := s.buildsMissing(); len(got) != 1 {
		t.Fatalf("with no override, want the node reported missing, got %v", got)
	}

	t.Setenv(firmware.EnvNativeBinary, dir)
	if got := s.buildsMissing(); len(got) != 0 {
		t.Errorf("an override supplies this node, so nothing is missing; got %v", got)
	}
}

// A board image is not a native build, so a native override says nothing about
// a node that runs under an emulator: it would still have nothing to boot.
func TestANativeOverrideDoesNotAnswerForABoardNode(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, firmware.NativeBinaryName("simple_repeater"))
	if err := os.WriteFile(bin, []byte("#!/bin/true\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(firmware.EnvNativeBinary, dir)

	node := repeaterNode("Emu")
	node.Firmware = scenario.FirmwareRef{
		Version: "repeater-v0.0.0-not-a-release", Board: "Generic_E22_sx1262",
	}
	s := &Sim{nodes: []scenario.Node{node}}

	if got := s.buildsMissing(); len(got) != 1 {
		t.Errorf("a board node needs its image, not a host binary; got %v", got)
	}
}
