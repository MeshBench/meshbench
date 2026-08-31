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

// Stopping a node that cannot be reaped must end.
//
// The kill is not the hard part - nothing survives SIGKILL. What outlives the
// process is os/exec's own copy of its output: the node's log is whatever the
// caller handed the backend, exec copies stderr onto it from a goroutine, and
// cmd.Wait waits for that goroutine. A write that does not return therefore
// holds the wait open with the process long gone, and the wait after the kill
// had no deadline at all.
//
// That is not a hypothetical writer. The engine gives every native node a file
// on whatever filesystem the operator keeps their cache on, and a stalled NFS
// mount is precisely a write that does not return. One of those froze the
// whole teardown, on the frame thread, with no way out.
func TestStopGivesUpOnAChildThatWillNotBeReaped(t *testing.T) {
	t.Setenv(fakenative.EnvMode, fakenative.ModeStuck)
	// Released at the end, so the abandoned copy goroutine is not left blocked
	// for the rest of the run.
	release := make(chan struct{})
	defer close(release)

	n := &native.Native{Path: fakenative.Path(), Log: blockingWriter{release}}
	if err := n.Start(context.Background(), "127.0.0.1:1"); err != nil {
		t.Fatal(err)
	}

	// On a goroutine, because the failure being guarded against is a stop that
	// never returns, and a test that simply called it would hang rather than
	// fail.
	done := make(chan error, 1)
	go func() { done <- n.Stop() }()
	select {
	case err := <-done:
		// And it says so. Returning nil here would report a node nothing can
		// account for as one that stopped cleanly, which is the answer that
		// sends somebody looking somewhere else.
		if err == nil {
			t.Fatal("a node that was never reaped was reported as cleanly stopped")
		}
		t.Log(err)
	case <-time.After(30 * time.Second):
		t.Fatal("Stop never came back from a node whose output could not be drained")
	}
}

// A node that fell over and a node that was asked to stop are different
// findings, and the backend reported both as success.
//
// It matters most where nobody is watching. A repeater that dies at minute
// three of a two-hour run is a mesh with a hole in it from then on, and the
// only thing that ever knew was cmd.Wait's return value - which was discarded
// on the spot. Every result after that is about a different network from the
// one the scenario describes.
func TestANodeThatDiedIsNotACleanStop(t *testing.T) {
	n, br, log := startFake(t, fakenative.ModeCrash)
	waitSaid(t, log, "MeshCore up")

	if err := br.Close(); err != nil {
		t.Fatal(err)
	}
	err := n.Stop()
	if err == nil {
		t.Fatal("a node that exited with a failing status was reported as cleanly stopped")
	}
	// The status itself, because "it stopped badly" sends somebody to the logs
	// and "exit status 3" is often the whole answer.
	if want := strconv.Itoa(fakenative.CrashStatus); !strings.Contains(err.Error(), want) {
		t.Errorf("the stop error does not carry the exit status %s: %v", want, err)
	}
}

// The other half of the same rule: a node this backend killed itself is not a
// node that crashed. Without this, the change above would turn every teardown
// of a node that outsat its grace period into a report of a firmware failure -
// which is the same lie the other way round.
func TestANodeThisBackendKilledIsNotReportedAsACrash(t *testing.T) {
	t.Setenv(fakenative.EnvMode, fakenative.ModeStuck)
	n := &native.Native{Path: fakenative.Path()}
	if err := n.Start(context.Background(), "127.0.0.1:1"); err != nil {
		t.Fatal(err)
	}
	if err := n.Stop(); err != nil {
		t.Fatalf("stopping a node that had to be killed: %v", err)
	}
}
