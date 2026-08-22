// The clock, and the run.
package meshbench

import (
	"context"
	"time"
)

// Sim is the run. Live.
type Sim struct{ w *Workbench }

// Sim reaches the clock.
func (w *Workbench) Sim() Sim { return Sim{w} }

// State is what the clock is doing.
func (s Sim) State(ctx context.Context) (SimState, error) {
	var st SimState
	return st, s.w.CallInto(ctx, "sim.state", nil, &st)
}

// Start warms the links, starts firmware on every node, and plays.
func (s Sim) Start(ctx context.Context) error { return s.w.Do(ctx, "sim.start", nil) }

// Play, Pause and Toggle are the clock itself.
func (s Sim) Play(ctx context.Context) error   { return s.w.Do(ctx, "sim.play", nil) }
func (s Sim) Pause(ctx context.Context) error  { return s.w.Do(ctx, "sim.pause", nil) }
func (s Sim) Toggle(ctx context.Context) error { return s.w.Do(ctx, "sim.toggle", nil) }

// Step advances one tick, which is StepMs of simulated time.
func (s Sim) Step(ctx context.Context) error { return s.w.Do(ctx, "sim.step", nil) }

// Reset puts the clock and the counters back to the start of the run.
func (s Sim) Reset(ctx context.Context) error { return s.w.Do(ctx, "sim.reset", nil) }

// SetSeed fixes the run. Same seed, same scenario, same result - which is what
// makes a *changed* result mean something.
func (s Sim) SetSeed(ctx context.Context, seed uint64) error {
	return s.w.Do(ctx, "sim.seed", map[string]any{"seed": float64(seed)})
}

// SetStepMs is how much simulated time one tick advances.
func (s Sim) SetStepMs(ctx context.Context, ms uint32) error {
	return s.w.Do(ctx, "sim.speed", map[string]any{"step_ms": float64(ms)})
}

// SetRealFirmware chooses whether play starts MeshCore on every node, or runs
// the channel with nothing behind it.
func (s Sim) SetRealFirmware(ctx context.Context, on bool) error {
	return s.w.Do(ctx, "sim.kind", map[string]any{"real": on})
}

// Run advances the mesh's own clock by this much and waits for it to finish.
//
// Simulated time, not yours. Five minutes here is five minutes of the mesh's
// clock; on 155 emulated nodes that is a great deal longer than five of yours,
// which is why the wait's own timeout is separate and generous.
func (s Sim) Run(ctx context.Context, simulated, wait time.Duration) error {
	ms := simulated.Milliseconds()
	if err := s.w.Do(ctx, "sim.run", map[string]any{"for_ms": float64(ms)}); err != nil {
		return err
	}
	return s.WaitStopped(ctx, wait)
}

// Settle steps the engine on a paused run, which is how a command gets the
// time it needs to be answered without starting the clock.
func (s Sim) Settle(ctx context.Context, steps int) error {
	return s.w.Do(ctx, "sim.settle", map[string]any{"steps": float64(steps)})
}

// WaitStopped waits for a run to end.
func (s Sim) WaitStopped(ctx context.Context, timeout time.Duration) error {
	return waitFor(ctx, timeout, "the run to finish", func() (bool, string, error) {
		st, err := s.State(ctx)
		if err != nil {
			return false, "", err
		}
		if !st.Playing {
			return true, "", nil
		}
		return false, secs(st.NowMs) + " of simulated time", nil
	})
}

// WaitUntil waits for the mesh's clock to reach a moment.
func (s Sim) WaitUntil(ctx context.Context, atMs uint32, timeout time.Duration) error {
	return waitFor(ctx, timeout, "simulated time to reach "+secs(atMs),
		func() (bool, string, error) {
			st, err := s.State(ctx)
			if err != nil {
				return false, "", err
			}
			if st.NowMs >= atMs {
				return true, "", nil
			}
			return false, secs(st.NowMs), nil
		})
}

// secs is a simulated moment, said the way a person reads it.
func secs(ms uint32) string {
	return (time.Duration(ms) * time.Millisecond).String()
}
