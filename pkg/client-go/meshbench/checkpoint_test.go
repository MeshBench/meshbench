package meshbench

import (
	"testing"
)

// A checkpoint is only worth anything if what comes back is what went in. The
// round trip is the test: build a network, move the clock, freeze it, throw the
// session away, restore, and the network and the moment are both back.
func TestCheckpointRoundTrip(t *testing.T) {
	wb, ctx := headless(t)
	if err := wb.Project().New(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := wb.Nodes().PlaceMany(ctx, []Placement{
		{Name: "R1", Kind: SimpleRepeater, Lat: 56.20, Lon: -3.20},
		{Name: "R2", Kind: SimpleRepeater, Lat: 56.12, Lon: -3.02},
		{Name: "C1", Kind: Companion, Lat: 56.19, Lon: -3.17},
	}); err != nil {
		t.Fatal(err)
	}

	// Move the clock off zero without waiting on wall time - settle steps the
	// engine directly, which is deterministic and fast.
	if _, err := wb.Call(ctx, "sim.settle", map[string]any{"steps": 10.0}); err != nil {
		t.Fatalf("settle: %v", err)
	}
	var state struct {
		NowMs uint32 `json:"now_ms"`
	}
	if err := wb.CallInto(ctx, "sim.state", nil, &state); err != nil {
		t.Fatal(err)
	}
	if state.NowMs == 0 {
		t.Fatal("the clock did not move; the rest of the test is meaningless")
	}

	cp, err := wb.Checkpoint(ctx, "trip-1")
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if cp.Nodes != 3 {
		t.Fatalf("checkpoint says %d nodes, want 3", cp.Nodes)
	}
	if cp.NowMs != state.NowMs {
		t.Fatalf("checkpoint froze the clock at %d, want %d", cp.NowMs, state.NowMs)
	}
	if cp.Path == "" {
		t.Fatal("checkpoint returned no path")
	}

	names, err := wb.Checkpoints(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !hasName(names, "trip-1") {
		t.Fatalf("the checkpoint is not listed: %v", names)
	}

	// Throw the session away, then bring it back.
	if err := wb.Project().New(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if list, _ := wb.Nodes().List(ctx); len(list) != 0 {
		t.Fatalf("a fresh project still holds %d nodes", len(list))
	}

	r, err := wb.Restore(ctx, "trip-1")
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if r.Nodes != 3 {
		t.Fatalf("restore brought back %d nodes, want 3", r.Nodes)
	}
	if r.TargetMs != cp.NowMs {
		t.Fatalf("restore is replaying to %d, want the checkpoint's %d", r.TargetMs, cp.NowMs)
	}
	if !r.Replaying {
		t.Fatal("a checkpoint taken off zero should restore by replaying to it")
	}

	// The network is actually back, not just counted.
	list, err := wb.Nodes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("after restore the network holds %d nodes, want 3", len(list))
	}
	byName := map[string]bool{}
	for _, n := range list {
		byName[n.Name] = true
	}
	for _, want := range []string{"R1", "R2", "C1"} {
		if !byName[want] {
			t.Errorf("restore lost node %s", want)
		}
	}
}

func hasName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}
