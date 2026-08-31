// What the backend does around a child process, checked without one.
//
// These need no MeshCore build, no emulator and no environment variable: the
// test binary stands in for the node, so the whole start-run-stop path runs on
// every machine and in the pipeline, which none of it did before.
package native_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/firmware/fakenative"
	"github.com/MeshBench/meshbench/internal/firmware/native"
)

func TestAStandInNodeAttachesTicksAndStopsCleanly(t *testing.T) {
	n, br, log := startFake(t, fakenative.ModeAttach)

	waitBridgeAttached(t, br)
	if n.PID() == 0 {
		t.Fatal("a running node reported no process")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := br.Advance(ctx, 10); err != nil {
		t.Fatalf("advance: %v", err)
	}

	// The bridge first, then the node: dropping the socket is how a node is
	// told to stop, and the closing line is the evidence it was told rather
	// than killed.
	if err := br.Close(); err != nil {
		t.Fatal(err)
	}
	if err := n.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	log.saidAll(t, fakenative.BootLine, fmt.Sprintf(fakenative.ClosedLine, 10))
}

// A second Start on a running node must be refused, and must leave the running
// one exactly as it was. Overwriting it would lose the only handle to the first
// process, which then survives every stop the caller can make.
func TestStartIsRefusedOnANodeAlreadyRunning(t *testing.T) {
	n, br, _ := startFake(t, fakenative.ModeAttach)
	waitBridgeAttached(t, br)

	first := n.PID()
	if err := n.Start(context.Background(), br.Addr()); err == nil {
		t.Fatal("starting a running node was allowed")
	}
	if got := n.PID(); got != first {
		t.Fatalf("the refused start moved the process from %d to %d", first, got)
	}
}

// A launch that failed is not a node, and the backend must not go on to hold
// one. Stop then has nothing to do, and saying so is what lets a caller unwind
// a half-built scenario without checking what failed where.
func TestAFailedLaunchLeavesNothingRunning(t *testing.T) {
	// A file that exists and will not execute. An explicit path is only
	// stat'd, so this gets past resolution and fails at the launch itself -
	// the case a missing binary never reaches, and the one a caller sees when
	// a download landed but arrived without its execute bit.
	path := filepath.Join(t.TempDir(), "not-a-node")
	if err := os.WriteFile(path, []byte("#not a binary\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	n := &native.Native{Path: path}
	if err := n.Start(context.Background(), "127.0.0.1:1"); err == nil {
		t.Fatal("launching an unexecutable file was reported as a started node")
	}
	if n.PID() != 0 {
		t.Errorf("a node that never launched reported process %d", n.PID())
	}
	if err := n.Stop(); err != nil {
		t.Errorf("stopping a node that never launched: %v", err)
	}
}

// Start reports that the process exists, not that it is running - and a node
// that exits the instant it is launched satisfies the first and not the
// second. Nothing here can tell the two apart, which is why the engine has an
// attach wait of its own; what this pins is that the backend survives it and
// stops without complaint.
func TestANodeThatExitsAtOnceIsAStartedLaunchAndNoNode(t *testing.T) {
	n, br, log := startFake(t, fakenative.ModeExit)

	waitSaid(t, log, fakenative.BootLine)
	if err := br.Close(); err != nil {
		t.Fatal(err)
	}
	if err := n.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if br.Attached() {
		t.Error("a node that exited before connecting is attached to the bridge")
	}
}

func waitBridgeAttached(t *testing.T, br *firmware.Bridge) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !br.Attached() {
		if time.Now().After(deadline) {
			t.Fatal("the stand-in node never connected to the bridge")
		}
		time.Sleep(2 * time.Millisecond)
	}
}
