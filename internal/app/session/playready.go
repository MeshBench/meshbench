// Bringing a mesh up and playing it in one call - what a script gets one
// chance to ask for, and sim.start's own play-button shape does not give it.
package session

import (
	"context"
	"fmt"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// pollInterval is how often PlayWhenReady checks whether a wait is over.
//
// A tenth of a second: these waits run from seconds to minutes - a warm, real
// firmware booting - so anything faster is a needless spin against a store
// that has real work to do.
const pollInterval = 100 * time.Millisecond

// defaultWarmWait and defaultFirmwareWait are what PlayWhenReady gives each
// stage when a caller does not name its own: generous, because a warm on a
// large scenario and real firmware on a great many nodes both measure in
// minutes, and a wait that gives up early reports a hang that was actually
// still working.
const (
	defaultWarmWait     = 30 * time.Minute
	defaultFirmwareWait = 10 * time.Minute
)

// PlayWhenReady brings real firmware up if the world wants it and none is
// running, waits for it, and only then plays.
//
// sim.start is the play button's own handler, and it answers in two presses
// on purpose: pressed once it starts firmware and returns without playing -
// playing immediately raced the store's ticker against nodes still attaching
// and wedged it - so a person presses it again once they are up. A flag given
// once on a command line only gets one press, and copying sim.start's checks
// into that caller would be the same guard maintained twice. So this applies
// the same refusal sim.start already does (firmwareStartBlocker) before the
// first process launches, then does the blocking wait sim.start's handler
// cannot: a handler runs on the store's own goroutine, and blocking it stalls
// every other verb for as long as firmware takes to boot. This runs on the
// caller's goroutine instead, the way a script driving the control socket
// already has to.
func (s *Sim) PlayWhenReady(ctx context.Context, st *state.Store, warm, firmware time.Duration) error {
	if warm <= 0 {
		warm = defaultWarmWait
	}
	if err := pollUntil(ctx, warm, func() (bool, error) {
		return !s.warming(), nil
	}); err != nil {
		return fmt.Errorf("waiting for the link matrix to finish warming: %w", err)
	}

	snap := st.Snapshot()
	if snap == nil {
		return fmt.Errorf("no simulation loaded")
	}
	// s.nodes, not snap.Nodes: the world's own node list is only ever filled
	// by committing an import, and a scenario built straight onto the engine -
	// which is what every non-interactive caller does - never touches it. That
	// gap is exactly the "56 of 58" bug in firmwareNodeCount's own comment:
	// comparing two different populations rather than the scenario's one.
	if snap.RealFirmware && s.eng != nil && s.firmwareNodeCount() > 0 {
		if err := s.firmwareStartBlocker(); err != nil {
			return err
		}
		if s.firmwareCount() < s.firmwareNodeCount() {
			if _, err := st.Do(ctx, "firmware.start", nil); err != nil {
				return err
			}
			if firmware <= 0 {
				firmware = defaultFirmwareWait
			}
			if err := pollUntil(ctx, firmware, func() (bool, error) {
				v, err := st.Do(ctx, "firmware.state", nil)
				if err != nil {
					return false, err
				}
				return firmwareUp(v)
			}); err != nil {
				return fmt.Errorf("waiting for firmware to come up: %w", err)
			}
		}
	}
	_, err := st.Do(ctx, "sim.play", nil)
	return err
}

// pollUntil checks every pollInterval until check says yes, ctx ends, or the
// timeout runs out.
func pollUntil(ctx context.Context, timeout time.Duration, check func() (bool, error)) error {
	deadline := time.Now().Add(timeout)
	for {
		done, err := check()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// firmwareUp reads firmware.state's answer.
//
// The verb is asked rather than the fields read directly, because the store's
// goroutine owns them and this runs on the caller's. That means the answer
// arrives as the wire's own shape, so it is checked rather than asserted: a
// verb that changed what it returns should stop this waiting, not panic it.
func firmwareUp(v any) (bool, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return false, fmt.Errorf("firmware.state answered %T, not a map", v)
	}
	starting, ok := m["starting"].(bool)
	if !ok {
		return false, fmt.Errorf("firmware.state gave no starting flag")
	}
	running, ok := m["running"].(int)
	if !ok {
		return false, fmt.Errorf("firmware.state gave no running count")
	}
	nodes, ok := m["nodes"].(int)
	if !ok {
		return false, fmt.Errorf("firmware.state gave no node count")
	}
	return !starting && running >= nodes, nil
}
