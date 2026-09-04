package workbench

import (
	"fmt"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/uitest"
)

// eventTail is the store's bounded tail after step frames of traffic: the same
// number of events every time, all of them different from the ones before.
func eventTail(step, n int) []state.Event {
	out := make([]state.Event, 0, n)
	for i := range n {
		at := uint32(step*n + i)
		out = append(out, state.Event{
			AtMs: at, Kind: "rx", Class: "received",
			From: "Abernethy Repeater", To: "Bishop Hill",
			PacketID: uint64(at) + 1, SNRdB: 4.5,
			Detail: fmt.Sprintf("packet %d", at),
		})
	}
	return out
}

// What the panel holds on to must not grow with the run.
//
// The events panel is the one somebody leaves open for four hours, and its
// per-row click state used to be the only thing in the process still holding
// every event that had scrolled out of the store's tail.
func TestEventRowsDoNotGrowWithTheRun(t *testing.T) {
	const tail = 200
	p := &eventsPanel{}
	snap := &state.Snapshot{}
	h := uitest.New(p.Draw, snap)

	after := 0
	for step := range 40 {
		snap.Events = eventTail(step, tail)
		snap.EventTotal = (step + 1) * tail
		snap.Counts = state.EventCounts{Received: snap.EventTotal}
		h.Frame()
		if step == 0 {
			after = len(p.rows)
		}
	}
	if after == 0 {
		t.Fatal("no rows were laid out, so this proves nothing about their growth")
	}
	if len(p.rows) > tail+rowSlack {
		t.Errorf("click state for %d rows after 40 frames of a %d-event tail, want at most %d",
			len(p.rows), tail, tail+rowSlack)
	}
}
