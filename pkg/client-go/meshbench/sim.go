// The clock, and the run.
package meshbench

import (
	"context"
	"fmt"
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

// Start brings the run up: waits out the warm, starts every node, and plays.
//
// Deliberately not one call to sim.start. That verb is the play button's own
// handler and answers four ways - it pauses if already playing, declines while
// links are being measured, or starts firmware and does not play - so a script
// pressing it once gets whichever of those the moment happens to be in.
//
// Worse, it only starts firmware when *no* node is running. Pin a build onto
// two nodes of a fifty-eight node fixture and it considers the mesh started,
// plays with fifty-six of them down, and says nothing.
//
// So this asks for the three things it actually wants, in order, and checks
// each one. Zero timeouts mean the usual ones.
func (s Sim) Start(ctx context.Context) error {
	return s.StartWithin(ctx, 0, 0)
}

// StartWithin is Start with the two waits named: how long to give the link
// measurement, and how long to give the firmware.
func (s Sim) StartWithin(ctx context.Context, warm, firmware time.Duration) error {
	if warm <= 0 {
		warm = 30 * time.Minute
	}
	// The links first. Nothing that follows means anything against a matrix
	// that is still being measured.
	if err := s.w.WaitIdle(ctx, warm); err != nil {
		return err
	}
	// Idle is not the same as measured. A warm that failed or was cancelled
	// finishes its own job row, so the wait above returns having waited for
	// nothing: no link was measured, and every study after this would answer
	// over free space.
	if st, err := s.State(ctx); err != nil {
		return err
	} else if !st.LinksMeasured && !st.Warming {
		note := st.Ground.Note
		if note == "" {
			note = "warm the links again before reading anything from this run"
		}
		return fmt.Errorf("no link has been measured, so nothing here can "+
			"reach anything. %s", note)
	}
	// Then every node that is not up, which firmware.start does and sim.start
	// does only when none of them are.
	st, err := s.w.Firmware().State(ctx)
	if err != nil {
		return err
	}
	if st.Running < st.Nodes {
		if err := s.w.Firmware().Start(ctx); err != nil {
			return err
		}
		if err := s.w.Firmware().WaitStarted(ctx, firmware); err != nil {
			return err
		}
	}
	// Then the clock, by its own name. Play cannot pause, which is the other
	// half of what made Start unusable from a script.
	now, err := s.State(ctx)
	if err != nil {
		return err
	}
	if now.Playing {
		return nil
	}
	return s.Play(ctx)
}

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
