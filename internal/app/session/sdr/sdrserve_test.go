package sdr_test

import (
	"context"
	"errors"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	_ "github.com/MeshBench/meshbench/internal/app/session/sdr"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// The SDR verbs moved out of session and hold their servers off the Sim through
// the DomainState seam. These pin that they still register and that their
// no-engine and unknown-node refusals survive the move - the paths reachable
// without standing up a real engine and rtl_tcp server.
func TestSDRServeRefusesWithNoEngine(t *testing.T) {
	store := state.New(10)
	sim := &session.Sim{}
	session.Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)

	_, err := store.Do(ctx, "sdr.serve", map[string]any{"node": "obs"})
	if err == nil {
		t.Fatal("sdr.serve with no engine must refuse")
	}
	if !errors.Is(err, session.ErrNoSimulation) {
		t.Fatalf("sdr.serve refused with %v, want ErrNoSimulation", err)
	}
}

func TestSDRStopRefusesAnUnservedNode(t *testing.T) {
	store := state.New(10)
	sim := &session.Sim{}
	session.Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)

	if _, err := store.Do(ctx, "sdr.stop", map[string]any{"node": "obs"}); err == nil {
		t.Fatal("sdr.stop on a node that is not being served must refuse")
	}
}
