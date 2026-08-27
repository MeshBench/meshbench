package capture_test

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	_ "github.com/MeshBench/meshbench/internal/app/session/capture"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// The capture verbs moved out of session; they still register, and each still
// refuses when there is no engine to capture from - the path reachable without
// standing up a network. Driven through the store, as a client would.
func TestCaptureVerbsRefuseWithNoNetwork(t *testing.T) {
	store := state.New(10)
	sim := &session.Sim{}
	session.Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)

	for _, verb := range []string{"capture.file", "capture.wireshark", "capture.stop"} {
		if _, err := store.Do(ctx, verb, nil); err == nil {
			t.Errorf("%s with no network must refuse", verb)
		}
	}
}
