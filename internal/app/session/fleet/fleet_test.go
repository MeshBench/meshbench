package fleet

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func newFleetTestSim(t *testing.T) *state.Store {
	t.Helper()
	st := state.New(10)
	s := &session.Sim{}
	session.Register(st, s)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go st.Run(ctx)
	return st
}

// The bug: a single Sim-level s.fleetPending meant a second fleet.send before
// the first had been answered overwrote the first's marks. The first
// command's own collector then read the *second* command's data when it woke
// up, and the second command's collector later found nothing left at all -
// which is what "fleet commands give no response" looked like from the
// panel. There is no shared field left to race on: each send now carries its
// own pending object straight to its own collector, so two in flight at once
// cannot touch each other's results. This proves each fleet.replies call
// reports the command it was actually given, however many others are also
// in flight.
func TestFleetRepliesReportsTheCommandItWasGivenNotAnotherOnesInFlight(t *testing.T) {
	st := newFleetTestSim(t)
	ctx := context.Background()

	first := &fleetPending{cmd: "advert", marks: map[string]int{"Alpha": 0}}
	second := &fleetPending{cmd: "status", marks: map[string]int{"Beta": 0}}

	// As if two sends had happened back to back: this is the exact call
	// collectFleet makes once it is done waiting, now carrying its own
	// pending object rather than reading one off the Sim.
	if _, err := st.Do(ctx, "fleet.replies", first); err != nil {
		t.Fatal(err)
	}
	snap := st.Snapshot()
	if snap.FleetCommand != "advert" {
		t.Fatalf("after the first collector finished, got %q, wanted %q",
			snap.FleetCommand, "advert")
	}

	if _, err := st.Do(ctx, "fleet.replies", second); err != nil {
		t.Fatal(err)
	}
	snap = st.Snapshot()
	if snap.FleetCommand != "status" {
		t.Fatalf("after the second collector finished, got %q, wanted %q - "+
			"the old design would have lost this one entirely",
			snap.FleetCommand, "status")
	}
	if len(snap.FleetReplies) != 1 || snap.FleetReplies[0].Node != "Beta" {
		t.Errorf("got %+v, wanted the second command's own node", snap.FleetReplies)
	}
}

// The order matters the other way too: whichever collector finishes *last*
// is the one the panel should be showing, and that has to hold even if the
// one that finishes last is the one that was sent first.
func TestFleetRepliesTheLastToFinishIsWhatIsShown(t *testing.T) {
	st := newFleetTestSim(t)
	ctx := context.Background()

	sentFirst := &fleetPending{cmd: "sent first, answers last", marks: map[string]int{"A": 0}}
	sentSecond := &fleetPending{cmd: "sent second, answers first", marks: map[string]int{"B": 0}}

	if _, err := st.Do(ctx, "fleet.replies", sentSecond); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Do(ctx, "fleet.replies", sentFirst); err != nil {
		t.Fatal(err)
	}
	if got := st.Snapshot().FleetCommand; got != "sent first, answers last" {
		t.Errorf("got %q, wanted whichever collector actually finished last", got)
	}
}

func TestFleetRepliesWithNilPendingIsANoOp(t *testing.T) {
	st := newFleetTestSim(t)
	res, err := st.Do(context.Background(), "fleet.replies", nil)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := res.(map[string]any)
	if !ok || m["replies"] != 0 {
		t.Errorf("got %v", res)
	}
	if got := st.Snapshot().FleetCommand; got != "" {
		t.Errorf("a nil pending must not touch FleetCommand, got %q", got)
	}
}

func TestFleetSendRefusesAnEmptyCommand(t *testing.T) {
	st := newFleetTestSim(t)
	if _, err := st.Do(context.Background(), "fleet.send",
		map[string]any{"command": "   "}); err == nil {
		t.Fatal("wanted a refusal for a blank command")
	}
}

func TestFleetSendRefusesWithNoNetworkLoaded(t *testing.T) {
	st := newFleetTestSim(t)
	if _, err := st.Do(context.Background(), "fleet.send",
		map[string]any{"command": "advert"}); err == nil {
		t.Fatal("wanted a refusal with no network built yet")
	}
}
