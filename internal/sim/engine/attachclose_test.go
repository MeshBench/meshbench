package engine_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/firmware/fakenative"
	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// TestMain lets this binary be re-entered as the nodes it starts.
//
// Read before the testing package parses flags: a stand-in is launched with
// MeshCore's own arguments, and a test binary handed --bridge would refuse to
// start at all.
func TestMain(m *testing.M) {
	if fakenative.Mode() != "" {
		os.Exit(fakenative.Serve())
	}
	os.Exit(m.Run())
}

// An attach is asynchronous and can outlive the network that asked for it: a
// project opened over another one, or a session shutting down, closes the
// engine while nodes are still being started. Whatever those workers finish
// starting has nobody left to tick it or shut it down, and a MeshCore process
// nobody owns keeps its memory and its sockets until the workbench itself
// exits - which on this machine was fifty-six of them at once.
//
// Judged on the processes, not on the engine's bookkeeping. Each stand-in
// prints when its bridge is taken away, so a node that was started and never
// closed is one that never said so, whichever side of the close it landed on.
func TestNoFirmwareOutlivesAnEngineClosedMidAttach(t *testing.T) {
	nodefs := t.TempDir()
	t.Setenv(fakenative.EnvMode, fakenative.ModeAttach)
	t.Setenv(firmware.EnvNativeBinary, fakenative.Path())
	t.Setenv(firmware.EnvNodeFS, nodefs)

	e := engine.New(flat{50}, engine.Config{
		FreqMHz: 869.618, SF: 10, BandwidthHz: 250e3, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417,
	})
	// More nodes than the attach runs at once, so some are still queued when
	// the engine goes away and others are midway through starting.
	const nodes = 16
	for i := 0; i < nodes; i++ {
		e.Add(node(fmt.Sprintf("mid-%02d", i), 56.0+float64(i)*0.02, -3.2, 10), nil)
	}

	attached := make(chan error, 1)
	go func() { attached <- e.AttachNative(context.Background(), 4417) }()

	// Closed the moment the first node's process is being launched, which is
	// the window the leak lives in: before that there is nothing to leak, and
	// after the last one has been handed over there is nothing left to race.
	waitForAStartingNode(t, nodefs)
	if err := e.Close(); err != nil {
		t.Fatalf("closing the engine mid-attach: %v", err)
	}
	<-attached

	running, booted := stillRunning(nodefs, 20*time.Second)
	if len(running) > 0 {
		t.Errorf("%d of %d firmware processes are still running with nobody owning them: %v",
			len(running), nodes, running)
	}
	// Otherwise the close landed before any process existed and this proved
	// nothing, which is a passing test that would go on passing through the
	// leak coming back.
	if booted == 0 {
		t.Error("no node ever booted, so nothing was closed mid-attach")
	}
}

// closedPrefix is the fixed half of what a stand-in prints on its way out,
// taken from the stand-in's own format string so a change there cannot leave
// this quietly matching nothing.
var closedPrefix = strings.SplitN(fakenative.ClosedLine, "%", 2)[0]

// waitForAStartingNode blocks until the attach has reached the point of
// launching a process for at least one node. The log is opened just before the
// launch, so its existence is the earliest observable moment inside the window.
func waitForAStartingNode(t *testing.T, nodefs string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if logs, _ := filepath.Glob(filepath.Join(nodefs, "*", "stderr.log")); len(logs) > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the attach never started a node")
		}
		time.Sleep(time.Millisecond)
	}
}

// stillRunning names the nodes that booted and were never told to stop, and
// says how many booted at all.
//
// A stand-in prints one line when it boots and another when its bridge is
// taken away, so a log holding the first and not the second is a process that
// is still sitting on an open socket waiting for ticks that will never come.
// Given a while, because a node stops when its bridge closes rather than the
// instant the attach returns.
func stillRunning(nodefs string, within time.Duration) (open []string, booted int) {
	deadline := time.Now().Add(within)
	for {
		open, booted = nil, 0
		logs, _ := filepath.Glob(filepath.Join(nodefs, "*", "stderr.log"))
		for _, p := range logs {
			b, err := os.ReadFile(p) //nolint:gosec // a path this test just made
			if err != nil || !strings.Contains(string(b), fakenative.BootLine) {
				continue
			}
			booted++
			if !strings.Contains(string(b), closedPrefix) {
				open = append(open, filepath.Base(filepath.Dir(p)))
			}
		}
		if len(open) == 0 || time.Now().After(deadline) {
			return open, booted
		}
		time.Sleep(20 * time.Millisecond)
	}
}
