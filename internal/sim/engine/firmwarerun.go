// Running the firmware, and surviving it when it stops.
//
// A node whose MeshCore process has died must not stop the clock for the
// other three hundred - the mesh carries on without it, and the run says so
// rather than hanging.
package engine

import (
	"context"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
)

// DefaultFirmwareTickTimeout is how long one node gets to acknowledge one
// tick before the run gives up on it.
//
// A healthy advance is a socket round trip and whatever a millisecond of
// MeshCore costs: microseconds when a node has a core to itself, and the
// nodes all advance at once, so this covers the whole mesh rather than one
// node at a time. Generous on purpose, for the same reason attachBudget is:
// this deadline exists to turn a node that will never answer into an error,
// not to police how quickly a loaded machine schedules it.
//
// The number matters far less than there being one. Without a deadline the
// wait can only end when the node answers, and the caller is the goroutine
// that owns the whole application: one node that never acks stopped every
// verb and every frame, permanently, with nothing said.
const DefaultFirmwareTickTimeout = 30 * time.Second

// ticked is one node this tick actually reached, and the instant it was asked
// for.
//
// Recorded rather than looked up again in the second pass. Both the node's
// Firmware and its BootOffsetMs are published by an attach running on its own
// goroutine, so re-reading them between the two passes waits either for a
// node that was never sent this tick or for an instant nobody asked it about
// - and an ack that was never going to come is what parks the caller.
type ticked struct {
	i    int
	name string
	fw   *firmware.Node
	atMs uint32
}

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
	targets := e.firmwareTargets(now)
	if len(targets) == 0 {
		return nil
	}
	// Every tick on the wire first, then the waits. The nodes advance in
	// parallel on their own cores, and this thread pays for the slowest one
	// instead of the sum — the difference between a 300-node scenario stepping
	// and a 300-node scenario saturating the machine.
	busy := e.channelBusy(now)
	sent := make([]ticked, 0, len(targets))
	for _, t := range targets {
		// What the channel sounds like here, before the node decides whether to
		// talk. MeshCore asks its radio this in Dispatcher::checkSend, and the
		// answer has to arrive before the tick it applies to - a node told after
		// the fact would be deciding on a channel that has already changed.
		if err := t.fw.Bridge.SetChannelBusy(busy[t.i]); err != nil {
			e.markFirmwareDown(t.name, "its radio would not take the channel state: "+err.Error())
			continue
		}
		if err := t.fw.Bridge.BeginAdvance(t.atMs); err != nil {
			e.markFirmwareDown(t.name, "the tick could not be sent to it: "+err.Error())
			continue
		}
		sent = append(sent, t)
	}
	for _, t := range sent {
		if err := e.waitTicked(ctx, t); err != nil {
			e.markFirmwareDown(t.name, err.Error())
			continue
		}
		// The radio reports how the firmware has configured it in the same
		// message that acknowledges the tick, so this is where a gain or
		// transmit-power change becomes the engine's: after the node that made
		// the change has finished making it, and before the next tick's channel
		// decisions are computed against it.
		e.ApplyRadioState(t.i, t.fw.Bridge.Stats())
	}
	return nil
}

// waitTicked waits for one node's acknowledgement, under a deadline.
//
// Its own function so the cancel runs at the end of this wait rather than at
// the end of the tick: deferring inside the loop would hold three hundred
// timers open until every node had answered.
func (e *Engine) waitTicked(ctx context.Context, t ticked) error {
	wctx, cancel := context.WithTimeout(ctx, e.firmwareTickTimeout())
	defer cancel()
	return t.fw.Bridge.WaitAdvance(wctx, t.atMs)
}

func (e *Engine) firmwareTickTimeout() time.Duration {
	if e.Config.FirmwareTickTimeout > 0 {
		return e.Config.FirmwareTickTimeout
	}
	return DefaultFirmwareTickTimeout
}

// firmwareTargets is who this tick has to run: the node's index, its bridge,
// and the instant its own clock should reach.
//
// Read in one pass under the lock, because an attach publishes a node's
// Firmware and its boot offset from another goroutine and the two are one
// fact. Read separately - or a second time, later in the tick - they can be
// seen half applied: a node whose bridge is set but whose offset is not yet
// is asked to reach one instant and waited for at another, and the ack for
// an instant nobody asked about never arrives.
func (e *Engine) firmwareTargets(now uint32) []ticked {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]ticked, 0, len(e.nodes))
	for i, n := range e.nodes {
		if n.Firmware == nil {
			continue
		}
		name := n.specRef().Name
		if e.firmwareDown[name] {
			continue
		}
		// Each node's own clock: the run's time plus how long it had already
		// been powered on when the run began.
		out = append(out, ticked{i: i, name: name, fw: n.Firmware, atMs: now + n.BootOffsetMs})
	}
	return out
}

// FirmwareFailure is one node that stopped answering, and what was seen.
//
// The reason travels with the name because the two failures look identical
// from the outside and mean different things: a process that has gone is a
// node to restart, and a node that answered nothing inside its deadline is a
// machine that is overloaded or a firmware that is stuck.
type FirmwareFailure struct {
	Name string
	Why  string
}

// markFirmwareDown records that a node's firmware has stopped answering, the
// first time it is seen - later ticks skip it rather than paying for another
// failed round trip to learn the same thing again.
func (e *Engine) markFirmwareDown(name, why string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.firmwareDown == nil {
		e.firmwareDown = map[string]bool{}
	}
	if e.firmwareDown[name] {
		return
	}
	e.firmwareDown[name] = true
	e.firmwareNewlyDown = append(e.firmwareNewlyDown, FirmwareFailure{Name: name, Why: why})
}

// FirmwareFailures returns the nodes whose firmware has stopped answering
// since the last call, and clears them - drain semantics, so a caller
// polling on a timer reports each failure exactly once rather than on every
// tick it remains true.
func (e *Engine) FirmwareFailures() []FirmwareFailure {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.firmwareNewlyDown
	e.firmwareNewlyDown = nil
	return out
}
