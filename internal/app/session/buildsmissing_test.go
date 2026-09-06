package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// A node with nothing pinned is reported however the machine is set up.
//
// An override would in fact start it, but "no build chosen" is a gap in the
// scenario rather than in the cache, and answering it from the environment
// made the same fixture refuse on one machine and play on another. The nightly
// found that: it sets MESHBENCH_NATIVE, so a fixture whose nodes pin nothing
// stopped being a half mesh there and nowhere else.
func TestANodeWithNothingPinnedIsReportedEvenUnderAnOverride(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, firmware.NativeBinaryName("simple_repeater"))
	if err := os.WriteFile(bin, []byte("#!/bin/true\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(firmware.EnvNativeBinary, dir)

	s := &Sim{nodes: []scenario.Node{repeaterNode("Unpinned")}}
	got := s.buildsMissing()
	if len(got) != 1 {
		t.Fatalf("a node with no version pinned was not reported: %v", got)
	}
	if !strings.Contains(got[0], "no version pinned") {
		t.Errorf("the report does not say what is missing: %q", got[0])
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

// The refusal's count is the number of nodes missing a build, not the length
// of the shortened list it quotes.
//
// It counted the list after truncation, so any mesh past the threshold was
// refused with "no firmware for 5 of 58 nodes ... and 52 more" - a sentence
// that disagrees with itself, and understates the gap in the one direction
// that reads as ignorable. A fresh machine is the case where it matters most:
// nothing is cached, so the answer is every node.
func TestTheRefusalCountsNodesRatherThanTheQuotedFew(t *testing.T) {
	var nodes []scenario.Node
	for i := range 20 {
		nodes = append(nodes, repeaterNode(fmt.Sprintf("R%02d", i)))
	}
	s := &Sim{nodes: nodes}

	err := s.firmwareStartBlocker()
	if err == nil {
		t.Fatal("twenty nodes with nothing pinned did not block the run")
	}
	got := err.Error()
	if !strings.Contains(got, "no firmware for 20 of 20 nodes") {
		t.Errorf("the count is not the number of nodes:\n%s", got)
	}
	// And the quoted list is still short, with the rest counted.
	if !strings.Contains(got, "and 16 more") {
		t.Errorf("the quoted list should name four and count the rest:\n%s", got)
	}
}

// The denominator is the nodes that could run firmware, not the whole fleet.
//
// An SDR observer boots nothing, so counting it made a Fife mesh say "of 58"
// about a sentence that is only ever about 56 of them.
func TestTheDenominatorSkipsNodesThatNeverBootFirmware(t *testing.T) {
	observer := repeaterNode("Watcher")
	observer.Kind = scenario.SDRObserver
	s := &Sim{nodes: []scenario.Node{repeaterNode("R1"), observer}}

	err := s.firmwareStartBlocker()
	if err == nil {
		t.Fatal("a node with nothing pinned did not block the run")
	}
	if got := err.Error(); !strings.Contains(got, "1 of 1 nodes") {
		t.Errorf("an observer runs no firmware and should not be counted:\n%s", got)
	}
}
