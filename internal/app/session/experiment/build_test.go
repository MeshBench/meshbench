package experiment

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A sweep records which build each role ran, so two arms that came back
// identical are not confused with two arms that never switched build. The
// record used to carry the arm, the seed and the measurements, and nothing
// about the firmware.
func TestACellRecordsTheBuildEachRoleRan(t *testing.T) {
	nodes := []scenario.Node{
		{Name: "hill", Kind: scenario.SimpleRepeater,
			Firmware: scenario.FirmwareRef{Version: "repeater-v1.16.0"}},
		{Name: "phone", Kind: scenario.Companion},
		// A second repeater on the same version is one build, not two.
		{Name: "hill2", Kind: scenario.SimpleRepeater,
			Firmware: scenario.FirmwareRef{Version: "repeater-v1.16.0"}},
		// An SDR observer runs no firmware and is not a build.
		{Name: "ear", Kind: scenario.SDRObserver},
	}
	// The arm pins the repeater and the companion build, which is what these
	// nodes run: WithFirmware writes the repeater version over every
	// firmware-running node that is not a companion.
	sweep := session.SweepArm{
		RepeaterVersion:  "repeater-v1.17.1",
		CompanionVersion: "companion-v1.17.1",
	}
	got := cellBuilds(nodes, sweep)

	want := []BuildRef{
		{Role: "companion_radio", Version: "companion-v1.17.1"},
		{Role: "simple_repeater", Version: "repeater-v1.17.1"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d builds, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Role != want[i].Role || got[i].Version != want[i].Version {
			t.Errorf("build %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	// An arm that pins nothing records each role's own scenario version, so a
	// control arm says what the control was.
	bare := cellBuilds(nodes, session.SweepArm{})
	var rep *BuildRef
	for i := range bare {
		if bare[i].Role == "simple_repeater" {
			rep = &bare[i]
		}
	}
	if rep == nil || rep.Version != "repeater-v1.16.0" {
		t.Errorf("a bare arm should keep the scenario version, got %+v", bare)
	}
}

// The file and its size are what tell two builds under one label apart: a
// local-main rebuilt in place is a different binary a week later, and a version
// string alone would call them equal.
func TestResolveBuildFillsInTheFileAndSize(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("LOCALAPPDATA", cache)
	t.Setenv(firmware.EnvNativeBinary, "")
	dir := filepath.Join(cache, "meshbench", "firmware", "native", "repeater-v1.17.1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, firmware.NativeBinaryName("simple_repeater"))
	if err := os.WriteFile(bin, []byte("#!/bin/true\nnot really a build"), 0o755); err != nil {
		t.Fatal(err)
	}
	b := BuildRef{Role: "simple_repeater", Version: "repeater-v1.17.1"}
	resolveBuild(context.Background(), &b)
	if b.File != firmware.NativeBinaryName("simple_repeater") {
		t.Errorf("file = %q, want the resolved binary's name", b.File)
	}
	if b.Bytes == 0 {
		t.Error("bytes is 0; the size that tells two builds apart was not recorded")
	}
	// A version the cache has not got resolves to nothing, and says so by
	// leaving the file blank rather than inventing one.
	missing := BuildRef{Role: "simple_repeater", Version: "nowhere-v0"}
	resolveBuild(context.Background(), &missing)
	if missing.File != "" || missing.Bytes != 0 {
		t.Errorf("an unresolvable build got a file: %+v", missing)
	}
}
