package session

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// This is issue #321: headless -play used to call sim.play directly, which
// only ever moves the clock. A fresh store wants real firmware by default, so
// that advanced simulated time over zero MeshCore processes and exited clean -
// a regression that broke firmware entirely would still have gone green.

// A half-started mesh must refuse before the first process launches, naming
// which nodes have no build, rather than tick quietly to "for" and exit 0.
func TestPlayWhenReadyRefusesAHalfMesh(t *testing.T) {
	st := state.New(10)
	s := &Sim{gpuAsked: true}
	Register(st, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	// repeaterNode pins no firmware, so buildsMissing reports both by name
	// whatever this machine happens to have cached.
	nodes := []scenario.Node{repeaterNode("Alpha"), repeaterNode("Beta")}
	nodes[1].Position.Lon += 0.001
	s.build(nodes, 869.618)

	err := s.PlayWhenReady(ctx, st, time.Second, time.Second)
	if err == nil {
		t.Fatal("PlayWhenReady started a run with no firmware for either node")
	}
	for _, want := range []string{"Alpha", "Beta", "half a mesh"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err.Error(), want)
		}
	}
	if snap := st.Snapshot(); snap.Playing {
		t.Fatal("the clock started despite the refusal")
	}
}

// The fallback the task allows when a real attach is impractical to drive in
// a unit test: a session with RealFirmware set and no engine at all. There is
// nothing to bring up, so PlayWhenReady has nothing to wait for and plays
// straight away - the same answer a bare sim.play always gave this case, so
// the fix must not have broken it.
func TestPlayWhenReadyWithNoNetworkJustPlays(t *testing.T) {
	st := state.New(10)
	s := &Sim{}
	Register(st, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	if snap := st.Snapshot(); !snap.RealFirmware {
		t.Fatal("a fresh store should default to RealFirmware, or this test proves nothing")
	}

	if err := s.PlayWhenReady(ctx, st, time.Second, time.Second); err != nil {
		t.Fatalf("PlayWhenReady with no network: %v", err)
	}
	if snap := st.Snapshot(); !snap.Playing {
		t.Fatal("PlayWhenReady returned without playing")
	}
}

// The full regression: real firmware, and the clock must not run ahead of it.
//
// Gated behind MESHBENCH_LIVE, the same convention every other test in this
// package that starts a real MeshCore process uses, because it depends on a
// native build already sitting in the firmware cache and a CI runner or a
// fresh checkout has no reason to have one.
func TestPlayWhenReadyStartsFirmwareBeforeTheClock(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1: this starts a real MeshCore process")
	}
	st := state.New(10)
	s := &Sim{gpuAsked: true}
	Register(st, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	node := repeaterNode("Solo")
	// Whatever native repeater build this machine has cached; buildsMissing
	// matches on the bare version regardless of role.
	node.Firmware = scenario.FirmwareRef{Version: "repeater-v1.17.0"}
	s.build([]scenario.Node{node}, 869.618)

	// Watched concurrently so a regression back to "clock first" is caught
	// even if PlayWhenReady's own return value would not show it: the bug
	// this exists for is a moment during the run, not only its outcome.
	badOrder := make(chan struct{}, 1)
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			if snap := st.Snapshot(); snap != nil && snap.Playing && snap.FirmwareRunning == 0 {
				select {
				case badOrder <- struct{}{}:
				default:
				}
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()

	playCtx, playCancel := context.WithTimeout(ctx, 90*time.Second)
	defer playCancel()
	err := s.PlayWhenReady(playCtx, st, 30*time.Second, 60*time.Second)
	close(stop)
	if err != nil {
		t.Fatalf("PlayWhenReady: %v", err)
	}

	select {
	case <-badOrder:
		t.Fatal("the clock was playing while no firmware was running")
	default:
	}

	snap := st.Snapshot()
	if !snap.Playing {
		t.Fatal("PlayWhenReady returned without playing")
	}
	if snap.FirmwareRunning == 0 {
		t.Fatal("PlayWhenReady returned playing with no firmware running")
	}
}
