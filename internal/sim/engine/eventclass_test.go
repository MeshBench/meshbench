package engine_test

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// Every miss a run produces, with the class the engine gave it.
func missClasses(t *testing.T, e *engine.Engine) map[engine.Class][]string {
	t.Helper()
	out := map[engine.Class][]string{}
	for _, ev := range e.Events() {
		if ev.Kind != "miss" {
			continue
		}
		c := engine.EventClass(ev)
		out[c] = append(out[c], ev.Detail)
	}
	return out
}

// A receiver whose demodulator was already following another preamble did not
// miss because the signal was weak, and must not be counted as though it did.
//
// It was counted as one for as long as the classifier read the detail
// sentence: the wording did not begin with any phrase it knew, so it fell
// through to "too quiet". An operator reading that card buys antennas for a
// mesh whose problem is traffic, which no antenna fixes.
func TestAReceiverLockMissIsNotAFloorMiss(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 4417})
	defer func() { _ = e.Close() }()
	e.Add(wfNode("listener", 0, 22), nil)
	e.Add(wfNode("east", 0.010, 22), nil)
	e.Add(wfNode("west", -0.010, 22), nil)

	frame := make([]byte, 40)
	for i := range frame {
		frame[i] = byte(90 + i)
	}
	if err := e.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	e.InjectFrame(1, frame)
	if err := e.Run(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	e.InjectFrame(2, frame) // arrives while the first is still being received
	if err := e.Run(context.Background(), 5000); err != nil {
		t.Fatal(err)
	}

	byClass := missClasses(t, e)
	if len(byClass[engine.ClassReceiverBusy]) == 0 {
		t.Fatalf("no miss was classed %q; classes seen: %v",
			engine.ClassReceiverBusy, classesOf(byClass))
	}
	for _, detail := range byClass[engine.ClassFloor] {
		t.Errorf("a miss on a clear, close path was classed %q: %q",
			engine.ClassFloor, detail)
	}
}

// A packet whose symbols a collision destroyed is a traffic problem, and the
// class has to say so rather than blaming the link.
func TestACollisionMissIsNotAFloorMiss(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 4417})
	defer func() { _ = e.Close() }()
	e.Add(wfNode("a", 0, 22), nil)
	e.Add(wfNode("b", 0.010, 22), nil)
	e.Add(wfNode("c", 0.020, 22), nil)

	frame := make([]byte, 40)
	for i := range frame {
		frame[i] = byte(37 + i*11)
	}
	if err := e.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	e.InjectFrame(0, frame)
	if err := e.Run(context.Background(), 200); err != nil {
		t.Fatal(err)
	}
	// Half a frame from an equal-power neighbour, landing mid-packet.
	e.InjectFrame(2, frame[:20])
	if err := e.Run(context.Background(), 8000); err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, ev := range e.Events() {
		if ev.Kind != "miss" || ev.From != "a" || ev.To != "b" {
			continue
		}
		found = true
		if got := engine.EventClass(ev); got != engine.ClassCollision {
			t.Errorf("a mid-packet collision was classed %q: %q", got, ev.Detail)
		}
	}
	if !found {
		t.Fatal("the collision produced no miss to classify")
	}
}

// Deafness keeps its own class, which is the one this set of classes started
// from: a node that could not listen is a timing problem, not a range one.
func TestHalfDuplexDeafnessKeepsItsOwnClass(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10})
	defer func() { _ = e.Close() }()
	e.Add(node("a", 56.700, -3.900, 22), nil)
	e.Add(node("b", 56.705, -3.900, 22), nil)

	e.Inject(0, []byte("hello from a"))
	e.Inject(1, []byte("hello from b"))
	if err := e.Run(context.Background(), 2000); err != nil {
		t.Fatal(err)
	}

	if len(missClasses(t, e)[engine.ClassHalfDuplex]) == 0 {
		t.Error("two nodes transmitting across each other produced no half-duplex class")
	}
}

// The counts the cards read are the classes the events carry, so a cause
// cannot be visible on a row and missing from the tally above it.
func TestEventCountsAgreeWithTheEventsThemselves(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 4417})
	defer func() { _ = e.Close() }()
	e.Add(wfNode("listener", 0, 22), nil)
	e.Add(wfNode("east", 0.010, 22), nil)
	e.Add(wfNode("west", -0.010, 22), nil)

	frame := make([]byte, 40)
	for i := range frame {
		frame[i] = byte(11 + i*5)
	}
	if err := e.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	e.InjectFrame(1, frame)
	e.InjectFrame(2, frame)
	if err := e.Run(context.Background(), 5000); err != nil {
		t.Fatal(err)
	}

	want := map[engine.Class]int{}
	for _, ev := range e.Events() {
		want[engine.EventClass(ev)]++
	}
	got := e.EventCounts()
	for _, c := range engine.Classes {
		if got[c] != want[c] {
			t.Errorf("EventCounts[%q] = %d, want %d", c, got[c], want[c])
		}
	}
	for c, n := range got {
		if want[c] != n {
			t.Errorf("EventCounts has %d of unlisted class %q", n, c)
		}
	}
}

// classesOf names the classes a run produced, for a failure message.
func classesOf(byClass map[engine.Class][]string) []engine.Class {
	var out []engine.Class
	for c := range byClass {
		out = append(out, c)
	}
	return out
}
