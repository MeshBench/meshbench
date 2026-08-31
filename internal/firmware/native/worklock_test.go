// One process per working directory, checked without a MeshCore build.
//
// docs/development-machines.md records "one at a time" as a table entry, and
// project history has a same-named node's second run corrupting the first's
// flash.bin and radio.sock silently. This is the code guarantee behind that
// table entry: a second node against a directory already in use is refused,
// not raced.
package native_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/MeshBench/meshbench/internal/firmware/fakenative"
	"github.com/MeshBench/meshbench/internal/firmware/native"
)

func TestSecondNodeInTheSameWorkDirIsRefused(t *testing.T) {
	// ModeExit needs no bridge to dial: the process starts, exits at once, and
	// what is under test is the work dir, not the node's own lifecycle.
	t.Setenv(fakenative.EnvMode, fakenative.ModeExit)
	dir := filepath.Join(t.TempDir(), "node-1")

	first := &native.Native{Path: fakenative.Path(), WorkDir: dir}
	if err := first.Start(context.Background(), "127.0.0.1:1"); err != nil {
		t.Fatalf("first start: %v", err)
	}
	t.Cleanup(func() { _ = first.Stop() })

	second := &native.Native{Path: fakenative.Path(), WorkDir: dir}
	err := second.Start(context.Background(), "127.0.0.1:1")
	if err == nil {
		_ = second.Stop()
		t.Fatal("a second node against a directory already in use was allowed to start")
	}
	if second.PID() != 0 {
		t.Errorf("a refused start still reported process %d", second.PID())
	}
}

// The lock outlives Start: a node still being torn down may still be writing
// to WorkDir, so a second node must be refused until Stop has actually
// confirmed the first one gone, not merely been called.
func TestAWorkDirStaysLockedUntilTheFirstNodeIsConfirmedGone(t *testing.T) {
	t.Setenv(fakenative.EnvMode, fakenative.ModeStuck)
	dir := filepath.Join(t.TempDir(), "node-1")

	first := &native.Native{Path: fakenative.Path(), WorkDir: dir}
	if err := first.Start(context.Background(), "127.0.0.1:1"); err != nil {
		t.Fatalf("first start: %v", err)
	}
	if err := first.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}

	second := &native.Native{Path: fakenative.Path(), WorkDir: dir}
	err := second.Start(context.Background(), "127.0.0.1:1")
	if err != nil {
		t.Fatalf("a work dir whose first node was confirmed stopped still refused a second: %v", err)
	}
	_ = second.Stop()
}

// A directory freed by a clean stop must be usable again - the ordinary case,
// exercised because the lock existing at all must not turn every second run of
// the same node into a refusal.
func TestAWorkDirIsReusableAfterACleanStop(t *testing.T) {
	t.Setenv(fakenative.EnvMode, fakenative.ModeExit)
	dir := filepath.Join(t.TempDir(), "node-1")

	first := &native.Native{Path: fakenative.Path(), WorkDir: dir}
	if err := first.Start(context.Background(), "127.0.0.1:1"); err != nil {
		t.Fatalf("first start: %v", err)
	}
	if err := first.Stop(); err != nil {
		t.Fatalf("first stop: %v", err)
	}

	second := &native.Native{Path: fakenative.Path(), WorkDir: dir}
	if err := second.Start(context.Background(), "127.0.0.1:1"); err != nil {
		t.Fatalf("second start after a clean stop: %v", err)
	}
	if err := second.Stop(); err != nil {
		t.Fatalf("second stop: %v", err)
	}
}

// A launch that never gets as far as a running process must not leave the
// directory claimed - the caller has nothing to Stop, so nothing else would
// ever release it.
func TestAFailedLaunchReleasesTheWorkDirLock(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "node-1")

	// An explicit path that will not execute reaches the launch itself rather
	// than being refused at resolution, same as TestAFailedLaunchLeavesNothingRunning.
	badPath := filepath.Join(t.TempDir(), "not-a-node")
	if err := os.WriteFile(badPath, []byte("#not a binary\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	first := &native.Native{Path: badPath, WorkDir: dir}
	if err := first.Start(context.Background(), "127.0.0.1:1"); err == nil {
		t.Fatal("launching an unexecutable file was reported as a started node")
	}

	t.Setenv(fakenative.EnvMode, fakenative.ModeExit)
	second := &native.Native{Path: fakenative.Path(), WorkDir: dir}
	if err := second.Start(context.Background(), "127.0.0.1:1"); err != nil {
		t.Fatalf("a failed launch left the work dir locked: %v", err)
	}
	_ = second.Stop()
}
