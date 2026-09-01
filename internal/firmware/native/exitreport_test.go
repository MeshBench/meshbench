package native_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware/fakenative"
	"github.com/MeshBench/meshbench/internal/firmware/native"
)

// Exited is the signal a caller polling for a connection needs: closed the
// moment the process is gone, well before Stop is ever asked to do anything.
func TestExitedClosesAssoonAsAStandInIsGone(t *testing.T) {
	n, _, log := startFake(t, fakenative.ModeExit)
	waitSaid(t, log, fakenative.BootLine)

	select {
	case <-n.Exited():
	case <-time.After(10 * time.Second):
		t.Fatal("Exited never closed for a process that had already gone")
	}
	if err := n.ExitError(); err == nil {
		t.Error("ExitError was nil for a process that exited before anything attached")
	}
}

// A clean exit status is still reported: connecting was the point, and a
// process that left before that is a finding whatever it exited with.
func TestExitErrorNamesTheStatusEvenOnAZeroExit(t *testing.T) {
	n, _, log := startFake(t, fakenative.ModeExit)
	waitSaid(t, log, fakenative.BootLine)
	<-n.Exited()

	err := n.ExitError()
	if err == nil {
		t.Fatal("ExitError was nil")
	}
	if !strings.Contains(err.Error(), "0") {
		t.Errorf("ExitError() = %v, want it to name the exit status", err)
	}
}

// A failing exit status survives to ExitError exactly as it does to Stop's
// own account, because it is the same fact asked a different way.
func TestExitErrorNamesAFailingStatus(t *testing.T) {
	n, _, log := startFake(t, fakenative.ModeCrash)
	waitSaid(t, log, fakenative.BootLine)
	<-n.Exited()

	err := n.ExitError()
	if err == nil {
		t.Fatal("ExitError was nil for a node that exited with a failing status")
	}
	want := strconv.Itoa(fakenative.CrashStatus)
	if !strings.Contains(err.Error(), want) {
		t.Errorf("ExitError() = %v, want it to mention exit status %s", err, want)
	}
}

// StderrTail carries the boot line even when the caller gave this node no
// Log at all - the one place a launch that never connects can be explained
// from, when nobody happened to be watching.
func TestStderrTailIsKeptWithoutALog(t *testing.T) {
	t.Setenv(fakenative.EnvMode, fakenative.ModeExit)
	n := &native.Native{Path: fakenative.Path()}
	if err := n.Start(context.Background(), "127.0.0.1:1"); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = n.Stop() })

	select {
	case <-n.Exited():
	case <-time.After(10 * time.Second):
		t.Fatal("Exited never closed")
	}
	if !strings.Contains(n.StderrTail(), fakenative.BootLine) {
		t.Errorf("StderrTail() = %q, want it to contain %q", n.StderrTail(), fakenative.BootLine)
	}
}
