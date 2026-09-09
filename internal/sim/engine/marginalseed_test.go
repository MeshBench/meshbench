package engine_test

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// A link within a couple of dB of threshold decodes on some seeds and misses
// on others, because the noise floor fluctuates run to run. Without that draw
// the calculated path was a pure function of geometry: every seed gave the
// identical run, so a sweep's rx_spread was structurally zero and the number
// every experiment delta must beat could never be observed (#671).
func TestSeedsMoveAMarginalCalculatedLink(t *testing.T) {
	// ~75 km on flat ground at 14 dBm SF10: right at the demodulator's edge.
	const lon = 0.6757

	deliver := func(seed uint64) bool {
		e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: seed})
		defer func() { _ = e.Close() }()
		e.Add(node("a", 56.7, -3.9, 14), nil)
		e.Add(node("b", 56.7, -3.9+lon, 14), nil)
		frame := make([]byte, 24)
		for i := range frame {
			frame[i] = byte(i)
		}
		_ = e.Run(context.Background(), 10)
		e.InjectFrame(0, frame)
		_ = e.Run(context.Background(), 400)
		for _, ev := range e.Events() {
			if ev.Kind == "rx" && ev.To == "b" {
				return true
			}
		}
		return false
	}

	got := 0
	const seeds = 16
	for s := uint64(1); s <= seeds; s++ {
		if deliver(s) {
			got++
		}
	}
	// Neither always nor never: the seed moves the run.
	if got == 0 || got == seeds {
		t.Fatalf("a marginal link delivered on %d of %d seeds; the seed does not move "+
			"the run, so rx_spread is still structurally zero", got, seeds)
	}

	// And still deterministic: the same seed gives the same answer every time.
	first := deliver(7)
	for i := 0; i < 3; i++ {
		if deliver(7) != first {
			t.Fatal("one seed gave two answers; the run is no longer reproducible")
		}
	}
}
