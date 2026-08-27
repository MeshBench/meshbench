package resources_test

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	_ "github.com/MeshBench/meshbench/internal/app/session/resources"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// The resource verbs moved out of session; resource.list still registers and
// answers with a row count (even an empty cache is zero rows, not an error),
// and resource.remove still refuses something the inventory does not hold.
// Driven through the store, as a client would.
func TestResourceListAndRemove(t *testing.T) {
	store := state.New(10)
	sim := &session.Sim{}
	session.Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)

	got, err := store.Do(ctx, "resource.list", nil)
	if err != nil {
		t.Fatalf("resource.list: %v", err)
	}
	if _, ok := got.(map[string]any)["rows"]; !ok {
		t.Fatalf("resource.list returned %v, want a rows count", got)
	}

	if _, err := store.Do(ctx, "resource.remove", map[string]any{
		"kind": "nothing", "name": "nowhere",
	}); err == nil {
		t.Error("resource.remove accepted a resource the inventory does not hold")
	}
}
