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

// A run shorter than the panel starts under the header, not against the floor.
//
// Gio's ScrollToEnd is end alignment rather than "scroll to the end": with
// fewer rows than fit, it pushed them down by the whole of the leftover space
// and left the column header at the top of a band of nothing. On a fresh
// machine with a short run - which is what a new user has - that was most of
// the Inspector, and it reads as a panel that has not loaded.
func TestAShortRunStartsUnderTheHeader(t *testing.T) {
	p := &eventsPanel{}
	snap := &state.Snapshot{Events: eventTail(0, 3), EventTotal: 3}
	snap.Counts = state.EventCounts{Received: 3}
	h := uitest.New(p.Draw, snap)

	// Twice: the first frame is what teaches the list its own size.
	h.Frame()
	h.Frame()

	if p.list.Position.OffsetLast <= 0 {
		t.Skip("three rows filled the test viewport, so there is nothing to " +
			"align either way")
	}
	if p.list.ScrollToEnd {
		t.Error("the list is end-aligned with room to spare, so the rows sit " +
			"against the bottom edge and the header labels a band of nothing")
	}
	if p.list.Position.Offset < 0 {
		t.Errorf("the first row is pushed down by %d px of blank space",
			-p.list.Position.Offset)
	}
}

// A run longer than the panel still follows its newest row.
func TestALongRunStillFollowsTheNewestRow(t *testing.T) {
	p := &eventsPanel{}
	snap := &state.Snapshot{Events: eventTail(0, 400), EventTotal: 400}
	snap.Counts = state.EventCounts{Received: 400}
	h := uitest.New(p.Draw, snap)

	h.Frame()
	h.Frame()

	if p.list.Position.OffsetLast > 0 {
		t.Fatal("400 rows fit the test viewport, so this proves nothing")
	}
	if !p.list.ScrollToEnd {
		t.Error("more rows than fit, so the panel should still be following " +
			"the newest one")
	}
}
