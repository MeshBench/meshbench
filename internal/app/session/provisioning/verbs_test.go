package provisioning_test

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	_ "github.com/MeshBench/meshbench/internal/app/session/provisioning"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// provisioning.set changes a setting and provisioning.get reads it back, and
// provisioning.apply refuses with no network - the three verbs moved out of
// session into this package, driven through the store as a client would.
func TestProvisioningSetGetRoundTrip(t *testing.T) {
	store := state.New(10)
	sim := &session.Sim{}
	session.Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)

	// A default read, then flip a flag and a number, then read it back.
	if _, err := store.Do(ctx, "provisioning.get", nil); err != nil {
		t.Fatalf("provisioning.get: %v", err)
	}
	got, err := store.Do(ctx, "provisioning.set", map[string]any{
		"set_name": false, "advert_hops": float64(7),
	})
	if err != nil {
		t.Fatalf("provisioning.set: %v", err)
	}
	m, _ := got.(map[string]any)
	if m["advert_hops"] != 7 {
		t.Fatalf("after set, advert_hops = %v, want 7", m["advert_hops"])
	}
	if m["set_name"] != false {
		t.Fatalf("after set, set_name = %v, want false", m["set_name"])
	}
	// It sticks: a fresh get sees the change the set made.
	got, _ = store.Do(ctx, "provisioning.get", nil)
	if m2, _ := got.(map[string]any); m2["advert_hops"] != 7 {
		t.Fatalf("provisioning.get after set = %v, want 7", m2["advert_hops"])
	}
}

func TestProvisioningApplyRefusesWithNoNetwork(t *testing.T) {
	store := state.New(10)
	sim := &session.Sim{}
	session.Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)

	if _, err := store.Do(ctx, "provisioning.apply", nil); err == nil {
		t.Fatal("provisioning.apply with no network must refuse")
	}
}
