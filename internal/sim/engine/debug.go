package engine

import (
	"context"
	"errors"
	"time"
)

// Debugging a node's firmware while the run goes on.
//
// A native node is a process in lockstep with the engine: every tick waits for
// its acknowledgement. Stopped at a breakpoint it sends none, and the tick's
// deadline would mark it down - so a debugger could never be used on a node
// for longer than the deadline. A node being debugged is held instead: the
// tick it did not answer is remembered, the rest of the mesh carries on
// without it, and when the debugger lets it go it acknowledges, catches up to
// the current instant on its next tick and rejoins. Its clock jumps forward by
// however long it was held, which is what a real board stopped by a JTAG probe
// does too, and the run is no longer one a seed can repeat.
//
// An emulated board's CPU also gets the emulator's own GDB stub, which is the
// only way into it: the emulator is the process, and the firmware is inside.

// Debug is how a node is to be run for a debugger.
type Debug struct {
	// GDBPort is where an emulated board's GDB stub listens, on loopback.
	// Zero for a native node, which a debugger attaches to by its PID.
	GDBPort int
	// Wait holds an emulated board's CPU at reset until a debugger lets it
	// go, so a fault during boot can be caught.
	Wait bool
}

// debugGrace is how long a node being debugged gets to answer a tick before
// it is taken to be stopped in a debugger and held. Long enough that a node
// merely slow on a loaded machine is not held for nothing, which would move
// its clock; short enough that the window stays usable while it is stopped.
var debugGrace = 2 * time.Second

// SetDebug marks a node as being debugged, or with nil as not. It takes
// effect for an emulated board when its firmware next starts, and for a
// native node at once.
func (e *Engine) SetDebug(name string, d *Debug) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if d == nil {
		delete(e.debug, name)
		delete(e.held, name)
		return
	}
	if e.debug == nil {
		e.debug = map[string]Debug{}
	}
	e.debug[name] = *d
}

// Debugging reports whether a node is being debugged, and how.
func (e *Engine) Debugging(name string) (Debug, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	d, ok := e.debug[name]
	return d, ok
}

// Held reports whether a debugger is holding a node, and since which tick.
func (e *Engine) Held(name string) (uint32, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	at, ok := e.held[name]
	return at, ok
}

// stillHeld is true when a node being debugged has not yet answered the tick
// it was held at, and so is skipped this tick. Released, it rejoins.
func (e *Engine) stillHeld(t ticked) bool {
	e.mu.Lock()
	at, held := e.held[t.name]
	e.mu.Unlock()
	if !held {
		return false
	}
	if !t.fw.Bridge.CaughtUp(at) {
		return true
	}
	e.mu.Lock()
	delete(e.held, t.name)
	e.mu.Unlock()
	return false
}

// waitDebugged waits for a node being debugged, and holds it rather than
// failing it when it does not answer in time. Anything but a timeout - the
// process has gone - is still the failure it always was.
func (e *Engine) waitDebugged(ctx context.Context, t ticked) error {
	wctx, cancel := context.WithTimeout(ctx, debugGrace)
	defer cancel()
	err := t.fw.Bridge.WaitAdvance(wctx, t.atMs)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		return err
	}
	e.mu.Lock()
	if e.held == nil {
		e.held = map[string]uint32{}
	}
	e.held[t.name] = t.atMs
	e.mu.Unlock()
	return errHeld
}

var errHeld = errors.New("held by a debugger")
