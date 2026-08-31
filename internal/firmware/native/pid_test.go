package native_test

import (
	"context"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware/fakenative"
	"github.com/MeshBench/meshbench/internal/firmware/native"
)

// PID is read while the node it belongs to is starting and stopping.
//
// It is what an interface asks when somebody wants to know what a node costs,
// and with a hundred and fifty of these up that question is asked from a panel
// on the frame thread while the engine is bringing nodes up and taking them
// down on its own. Nothing serialises those, and nothing should have to.
//
// Under the race detector this fails outright if PID reads the process without
// the lock both writers hold; without the detector it is a torn read, which
// reports a process for a node that has none.
func TestPIDIsSafeToReadWhileTheNodeStartsAndStops(t *testing.T) {
	// A node that is gone as soon as it is launched, so the loop below is
	// start and stop rather than a wait: what is being raced is the backend's
	// own bookkeeping, not anything the child does.
	t.Setenv(fakenative.EnvMode, fakenative.ModeExit)

	n := &native.Native{Path: fakenative.Path()}
	stop := make(chan struct{})
	read := make(chan struct{})
	go func() {
		defer close(read)
		for {
			select {
			case <-stop:
				return
			default:
				_ = n.PID()
			}
		}
	}()

	// Several times round, because one start and one stop can interleave with
	// the reader by luck.
	for i := 0; i < 5; i++ {
		if err := n.Start(context.Background(), "127.0.0.1:1"); err != nil {
			t.Fatalf("start %d: %v", i, err)
		}
		if err := n.Stop(); err != nil {
			t.Fatalf("stop %d: %v", i, err)
		}
	}
	close(stop)
	select {
	case <-read:
	case <-time.After(10 * time.Second):
		t.Fatal("the reader never came back")
	}
}
