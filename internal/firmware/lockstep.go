package firmware

import (
	"context"
	"encoding/binary"
	"fmt"
)

// Advance runs the attached node forward to simulation time atMs and waits for
// it to say it got there.
//
// The wait is the point. A node left to free-run uses wall-clock time, so the
// order in which two nodes' transmissions reach the channel depends on host
// scheduling — and the same seed stops producing the same result, which
// CLAUDE.md rules out. Lockstep costs a round trip per tick and buys back
// reproducibility.
//
// It is also what makes a native node *faster* than real time rather than
// merely as fast: with the clock supplied rather than observed, a node that has
// nothing to do consumes a tick in microseconds.
func (b *Bridge) Advance(ctx context.Context, atMs uint32) error {
	var payload [4]byte
	binary.BigEndian.PutUint32(payload[:], atMs)
	if err := b.send(kindTick, payload[:]); err != nil {
		return fmt.Errorf("firmware: tick %s to %d ms: %w", b.node, atMs, err)
	}
	for {
		select {
		case got := <-b.acked:
			// A stale ack is a node that fell behind and caught up; a future one
			// is a node running its own clock, which breaks the determinism the
			// tick exists to provide.
			if got == atMs {
				return nil
			}
			if got > atMs {
				return fmt.Errorf("firmware: %s acked %d ms, ahead of the requested %d", b.node, got, atMs)
			}
		case <-ctx.Done():
			return fmt.Errorf("firmware: %s did not reach %d ms: %w", b.node, atMs, ctx.Err())
		}
	}
}

// TransmitFinished tells the node the waveform it started has ended on the air.
//
// The node must not work this out for itself. It can only *estimate* airtime —
// that is what the firmware's own getEstAirtimeFor() is for, and estimating is
// all the firmware does on real hardware too. How long the transmission
// actually occupied the channel is a property of the samples the engine
// generated, and the engine is the only thing that knows it.
//
// Letting the node time its own transmission would quietly replace the
// simulation with the formula, which is the same class of mistake as deciding
// collisions with a rule instead of letting them emerge from the sum.
func (b *Bridge) TransmitFinished() error {
	if err := b.send(kindTxDone, nil); err != nil {
		return fmt.Errorf("firmware: signal end of transmission to %s: %w", b.node, err)
	}
	return nil
}
