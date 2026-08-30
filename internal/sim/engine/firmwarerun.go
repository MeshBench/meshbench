// Running the firmware, and surviving it when it stops.
//
// A node whose MeshCore process has died must not stop the clock for the
// other three hundred - the mesh carries on without it, and the run says so
// rather than hanging.
package engine

import "context"

// runFirmware advances every node's firmware to the current instant.
//
// One node's firmware failing must not stop the others ticking. It used to:
// the first Bridge call that errored - a node whose process had crashed -
// returned out of this function immediately, which left every node after it
// in nodes never ticked again, on every subsequent call, forever, and the
// error was discarded by the caller (Step is called as `_ =
// s.eng.Step(...)`). Skipping the failed node and continuing is what a dead
// repeater actually means: it stops participating, the rest of the mesh does
// not.
func (e *Engine) runFirmware(ctx context.Context, now uint32) error {
	nodes := e.Nodes()
	// Every tick on the wire first, then the waits. The nodes advance in
	// parallel on their own cores, and this thread pays for the slowest one
	// instead of the sum — the difference between a 300-node scenario stepping
	// and a 300-node scenario saturating the machine.
	busy := e.channelBusy(now)
	for i, n := range nodes {
		if n.Firmware == nil || e.firmwareIsDown(n.specRef().Name) {
			continue
		}
		// What the channel sounds like here, before the node decides whether to
		// talk. MeshCore asks its radio this in Dispatcher::checkSend, and the
		// answer has to arrive before the tick it applies to - a node told after
		// the fact would be deciding on a channel that has already changed.
		if err := n.Firmware.Bridge.SetChannelBusy(busy[i]); err != nil {
			e.markFirmwareDown(n.specRef().Name)
			continue
		}
		// Each node's own clock: the run's time plus how long it had already
		// been powered on when the run began.
		if err := n.Firmware.Bridge.BeginAdvance(now + n.BootOffsetMs); err != nil {
			e.markFirmwareDown(n.specRef().Name)
			continue
		}
	}
	for i, n := range nodes {
		if n.Firmware == nil || e.firmwareIsDown(n.specRef().Name) {
			continue
		}
		if err := n.Firmware.Bridge.WaitAdvance(ctx, now+n.BootOffsetMs); err != nil {
			e.markFirmwareDown(n.specRef().Name)
			continue
		}
		// The radio reports how the firmware has configured it in the same
		// message that acknowledges the tick, so this is where a gain or
		// transmit-power change becomes the engine's: after the node that made
		// the change has finished making it, and before the next tick's channel
		// decisions are computed against it.
		e.ApplyRadioState(i, n.Firmware.Bridge.Stats())
	}
	return nil
}

// markFirmwareDown records that a node's firmware has stopped answering, the
// first time it is seen - later ticks skip it rather than paying for another
// failed round trip to learn the same thing again.
func (e *Engine) markFirmwareDown(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.firmwareDown == nil {
		e.firmwareDown = map[string]bool{}
	}
	if e.firmwareDown[name] {
		return
	}
	e.firmwareDown[name] = true
	e.firmwareNewlyDown = append(e.firmwareNewlyDown, name)
}

func (e *Engine) firmwareIsDown(name string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.firmwareDown[name]
}

// FirmwareFailures returns node names whose firmware has stopped answering
// since the last call, and clears them - drain semantics, so a caller
// polling on a timer reports each failure exactly once rather than on every
// tick it remains true.
func (e *Engine) FirmwareFailures() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.firmwareNewlyDown
	e.firmwareNewlyDown = nil
	return out
}
