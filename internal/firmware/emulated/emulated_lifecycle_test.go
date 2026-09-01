package emulated

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/firmware"
)

// A second Start on a node already running must be refused, not allowed to
// overwrite e.qemu and e.radio: doing so orphans the first pair, which
// nothing holds a reference to any more and Stop can then never reach.
func TestStartRefusesASecondCallWhileQEMUIsRunning(t *testing.T) {
	e := &EmulatedNode{qemu: exec.Command("sleep", "1")}
	err := e.Start(context.Background(), "")
	if err == nil {
		t.Fatal("a second Start on a node already running was allowed")
	}
	if !strings.Contains(err.Error(), "already started") {
		t.Errorf("the error does not say the node is already running: %v", err)
	}
}

func TestStartRefusesASecondCallWhileTheRadioModelIsRunning(t *testing.T) {
	e := &EmulatedNode{radio: exec.Command("sleep", "1")}
	err := e.Start(context.Background(), "")
	if err == nil {
		t.Fatal("a second Start on a node already running was allowed")
	}
	if !strings.Contains(err.Error(), "already started") {
		t.Errorf("the error does not say the node is already running: %v", err)
	}
}

// Two nodes sharing a working directory once corrupted each other silently -
// project history's own example is three hundred processes doing exactly
// this. A directory already claimed by another process must refuse a second
// node outright rather than let them share it.
func TestStartRefusesAWorkDirAlreadyLocked(t *testing.T) {
	dir := t.TempDir()
	lock, err := firmware.LockWorkDir(dir)
	if err != nil {
		t.Fatalf("claim the dir first: %v", err)
	}
	defer func() { _ = lock.Release() }()

	e := &EmulatedNode{Image: "placeholder", NodeName: "n1", Dir: dir}
	err = e.Start(context.Background(), "")
	if err == nil {
		t.Fatal("a work dir already claimed by another process was not refused")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Errorf("the error does not say the directory is in use: %v", err)
	}
}

// A launch that fails still owes the directory back: nothing else will ever
// call Stop on a node that never started, so nothing else will ever release
// the lock if Start does not.
func TestAFailedStartReleasesTheWorkDirLock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvRadioServer, filepath.Join(t.TempDir(), "not-here"))

	e := &EmulatedNode{Image: "placeholder", NodeName: "n1", Dir: dir}
	if err := e.Start(context.Background(), ""); err == nil {
		t.Fatal("started with no radio model present")
	}

	lock, err := firmware.LockWorkDir(dir)
	if err != nil {
		t.Fatalf("a failed start left the work dir locked: %v", err)
	}
	_ = lock.Release()
}
