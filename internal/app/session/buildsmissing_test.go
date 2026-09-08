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

// A pin is answered by role, not by its name. A repeater build pinned across
// a mesh with no role filter used to satisfy the companions too - the cache
// held the version, and the version was all that was asked - so the engine,
// which resolves by role, started eighteen of twenty-four nodes and the gate
// that exists to refuse a half mesh said nothing.
func TestAPinToAnotherRolesBuildIsReportedAsSuch(t *testing.T) {
	installed := []firmware.Installed{
		{Native: true, Role: "simple_repeater", Version: "repeater-v1.17.1"},
		{Native: false, Role: "companion_radio_usb", Version: "companion-v1.17.1", Board: "heltec_v3"},
	}
	if ok, _ := BuildAnswers(installed, "simple_repeater", "repeater-v1.17.1"); !ok {
		t.Error("a repeater's own build does not answer for it")
	}
	ok, others := BuildAnswers(installed, "companion_radio", "repeater-v1.17.1")
	if ok {
		t.Fatal("a repeater build answers for a companion, which is the fault")
	}
	if len(others) != 1 || others[0] != "simple_repeater" {
		t.Errorf("the refusal should name the role the build is for, got %v", others)
	}
	// A board image carries its transport in its role, and is the
	// companion's build all the same.
	if ok, _ := BuildAnswers(installed, "companion_radio", "companion-v1.17.1"); !ok {
		t.Error("a companion_radio_usb image does not answer for companion_radio")
	}
	if ok, others := BuildAnswers(installed, "companion_radio", "nowhere-v0"); ok || len(others) != 0 {
		t.Errorf("a version the cache has not got is neither answered nor blamed on a role, got %v %v", ok, others)
	}

	// And through the gate, against a real cache directory.
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
	t.Setenv(firmware.EnvNativeBinary, "")
	hill := repeaterNode("Hill")
	hill.Firmware = scenario.FirmwareRef{Version: "repeater-v1.17.1"}
	phone := scenario.Node{Name: "Phone", Kind: scenario.Companion,
		Firmware: scenario.FirmwareRef{Version: "repeater-v1.17.1"}}
	s := &Sim{nodes: []scenario.Node{hill, phone}}
	got := s.buildsMissing()
	if len(got) != 1 {
		t.Fatalf("the companion alone is missing a build, got %v", got)
	}
	for _, want := range []string{"Phone", "companion_radio", "repeater-v1.17.1", "simple_repeater build"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("the refusal does not say %q: %s", want, got[0])
		}
	}
}
