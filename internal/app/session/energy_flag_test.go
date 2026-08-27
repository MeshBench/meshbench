package session_test

import (
	"context"
	"errors"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// Energy monitoring is disabled until the model is trusted (#254): the verbs
// refuse with a reason rather than return a number nobody stands behind, and
// the refusal lifts only when the feature flag is set.
func TestEnergyVerbsRefuseUntilFlagged(t *testing.T) {
	store := state.New(10)
	sim := &session.Sim{}
	session.Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)

	// Off by default: both energy verbs refuse with ErrEnergyDisabled.
	t.Setenv("MESHBENCH_ENERGY", "")
	for _, v := range []string{"energy.for_selection", "node.energy"} {
		_, err := store.Do(ctx, v, "somewhere")
		if !errors.Is(err, session.ErrEnergyDisabled) {
			t.Errorf("%s off gave %v, want ErrEnergyDisabled", v, err)
		}
	}

	// Flagged on: the guard lifts. The verb still fails here - there is no node
	// selected - but with a different error, which proves it got past the gate.
	t.Setenv("MESHBENCH_ENERGY", "1")
	if _, err := store.Do(ctx, "energy.for_selection", nil); errors.Is(err, session.ErrEnergyDisabled) {
		t.Error("energy.for_selection still refused with the flag set")
	}
}
