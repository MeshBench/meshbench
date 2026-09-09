package engine_test

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// Waveform mode never named a collision: a miss above the noise floor with a
// stronger signal in the same window fell to unclassified, so scenario 4's
// "collision or capture verdict that calculated mode would not produce" could
// not appear - the full receiver was the one model that never said it. The
// summed window has the interferer's power; the classifier now reads it.
func TestWaveformNamesACollision(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 4417, RFMode: engine.RFWaveform})
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
	// An equal-power neighbour lands mid-packet: the wanted is loud on its own
	// and a signal within the capture margin is in the window with it.
	e.InjectFrame(2, frame[:20])
	if err := e.Run(context.Background(), 8000); err != nil {
		t.Fatal(err)
	}

	sawInterference := false
	for _, ev := range e.Events() {
		if ev.Kind != "miss" || ev.To != "b" {
			continue
		}
		if engine.EventClass(ev) == engine.ClassInterference {
			sawInterference = true
			if ev.Detail == "" {
				t.Error("an interference miss carried no detail")
			}
		}
	}
	if !sawInterference {
		t.Fatal("waveform mode produced no interference verdict for a mid-packet collision")
	}
}
