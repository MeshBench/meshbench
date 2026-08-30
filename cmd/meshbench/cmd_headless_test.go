package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/fixture"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Issue #321: "meshbench headless -play" used to call sim.play directly, which
// only ever moves the clock. A fresh session wants real firmware by default,
// so that advanced simulated time over zero MeshCore processes, produced no
// traffic, and exited 0 - a regression that broke firmware entirely would
// still have gone green in CI.

// writeFixture is the smallest fixture runHeadless will accept: fixture.Load
// refuses one with no nodes, and everything else defaults.
func writeFixture(t *testing.T, nodes []scenario.Node) string {
	t.Helper()
	f := fixture.Fixture{Name: "test", Nodes: nodes, Seed: 4417, FreqMHz: 869.618}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func repeaterAt(name string, lon float64) scenario.Node {
	return scenario.Node{
		Name: name, Kind: scenario.SimpleRepeater,
		Position: scenario.LatLon{Lat: 56.3, Lon: lon},
	}
}

// A fixture whose nodes have no firmware pinned must refuse to play - loudly,
// non-zero, naming the nodes - rather than tick simulated time over a mesh
// that was never there.
func TestHeadlessPlayRefusesAHalfMesh(t *testing.T) {
	path := writeFixture(t, []scenario.Node{repeaterAt("Alpha", -3.2), repeaterAt("Beta", -3.19)})
	sock := filepath.Join(t.TempDir(), "ctrl.sock")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	err := runHeadless(ctx, []string{
		"-fixture", path, "-play", "-for", "50ms",
		"-control-socket", sock, "-quiet",
	})
	if err == nil {
		t.Fatal("headless -play started a run against two nodes with no firmware pinned")
	}
	for _, want := range []string{"Alpha", "Beta", "half a mesh"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err.Error(), want)
		}
	}
}

// The full regression, end to end through the command line: real firmware
// comes up, and the status line says so, before the run is told it is
// playing.
//
// Gated behind MESHBENCH_LIVE like every other test in this repository that
// starts a real MeshCore process: it depends on a native build already
// sitting in the firmware cache, which a fresh checkout or a bare CI runner
// has no reason to have.
func TestHeadlessPlayStartsFirmwareBeforeTheClock(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1: this starts a real MeshCore process")
	}
	node := repeaterAt("Solo", -3.2)
	// Whatever native repeater build this machine has cached; buildsMissing
	// matches on the bare version regardless of role.
	node.Firmware = scenario.FirmwareRef{Version: "repeater-v1.17.0"}
	path := writeFixture(t, []scenario.Node{node})
	sock := filepath.Join(t.TempDir(), "ctrl.sock")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	realStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = realStderr }()

	captured := make(chan string, 1)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				sb.Write(buf[:n])
			}
			if err != nil {
				captured <- sb.String()
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	runErr := runHeadless(ctx, []string{
		"-fixture", path, "-play", "-for", "2s", "-control-socket", sock,
	})
	os.Stderr = realStderr
	_ = w.Close()
	out := <-captured
	_ = r.Close()

	if runErr != nil {
		t.Fatalf("runHeadless: %v\nstderr:\n%s", runErr, out)
	}

	firmwareAt := strings.Index(out, "running firmware")
	playingAt := strings.Index(out, "playing")
	if firmwareAt < 0 {
		t.Fatalf("no firmware status line seen; stderr:\n%s", out)
	}
	if playingAt < 0 {
		t.Fatalf("no \"playing\" status line seen; stderr:\n%s", out)
	}
	if playingAt < firmwareAt {
		t.Fatalf("the clock said \"playing\" before firmware was reported running:\n%s", out)
	}
}
