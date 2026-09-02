package firmware

import (
	"fmt"
	"io"
)

// Who owns an emulated node's serial port, and what goes over it.
//
// A node has one UART and the workbench has several things that want it - the
// console pane, a companion client over TCP, a script typing a command - so
// ownership is a single seat rather than a subscription. Splitting one UART
// between two readers gives each of them half the output, which is worse than
// giving one of them none.

// Console directs the node's serial output at w, and hands over what the node
// said while nobody was there to read it.
//
// Set rather than subscribed: a node has one serial port, and two readers of one
// UART would each see half the output - which is worse than not having it.
func (b *Bridge) Console(w io.Writer) {
	b.mu.Lock()
	// A claimed port belongs to whoever claimed it. Without this the workbench
	// console re-attached itself on the very next frame and quietly took the
	// UART back from an attached companion client - which looks exactly like a
	// client that connected and then received nothing.
	if b.claimed {
		b.mu.Unlock()
		return
	}
	b.console = w
	// Taken only when there is somewhere to put it. Detaching is Console(nil),
	// and clearing the scrollback on the way out would lose it to whoever
	// opened the console next.
	var held []byte
	node := b.node
	if w != nil {
		held, b.backlog = b.backlog, nil
	}
	b.mu.Unlock()
	if len(held) == 0 {
		return
	}
	// Named as scrollback rather than let through as live output. Every line
	// of it is about to be stamped with the clock as it arrives, which is now
	// and not when the node said it, and an unlabelled boot chain carrying the
	// current time is a worse answer than none.
	_, _ = w.Write([]byte("-- " + node + " said this before the console was opened --\n"))
	_, _ = w.Write(held)
}

// consoleBacklog is how much output a node keeps for a console nobody has
// opened yet.
//
// A node prints its version, its region and what its radio came up as in the
// first second, then goes quiet for minutes. The port had been attached to
// nobody for all of it, so every one of those lines was discarded on the way
// past and the console was empty exactly when somebody opened it to find out
// what had happened.
const consoleBacklog = 64 << 10

// writeConsole hands output to whoever holds the port, and keeps it for
// whoever opens it next when nobody does.
func (b *Bridge) writeConsole(p []byte) {
	if len(p) == 0 {
		return
	}
	b.mu.Lock()
	w := b.console
	if w == nil {
		b.backlog = append(b.backlog, p...)
		// Overflow goes from the front, as the console buffer's own does: what
		// a node said most recently is what its silence needs explaining by.
		if n := len(b.backlog) - consoleBacklog; n > 0 {
			b.backlog = append(b.backlog[:0], b.backlog[n:]...)
		}
	}
	b.mu.Unlock()
	if w != nil {
		// Best effort. A console that cannot be written to must not stall the
		// node it belongs to: the simulation's correctness does not depend on
		// anyone reading the output.
		_, _ = w.Write(p)
	}
}

// ConsoleSink is a writer that forwards to whoever currently holds this node's
// console, and discards when nobody does.
//
// For a backend that carries its own serial port rather than sending console
// frames over the bridge. The holder is looked up per write, because it
// changes while the node runs - the console pane attaches, a client claims the
// port, the claim is released - and a writer captured once would go on feeding
// whoever held it when the node booted, which is nobody.
func (b *Bridge) ConsoleSink() io.Writer { return bridgeConsole{b} }

type bridgeConsole struct{ b *Bridge }

func (c bridgeConsole) Write(p []byte) (int, error) {
	c.b.writeConsole(p)
	return len(p), nil
}

// Claim gives one owner exclusive use of the serial port, as a USB cable does.
//
// Returns a release function. While a claim is held, Console is ignored: two
// protocols interleaved on one UART is neither of them.
//
// The release only releases its own claim, once. An unconditional release
// let a stale holder clear whoever had claimed since - disconnect racing
// serve unplugged the TCP client that had just taken the port, which read as
// a client that connected and then received nothing. A generation number
// rather than comparing writers, because the same writer claiming again
// would make its old release match again.
func (b *Bridge) Claim(w io.Writer) func() {
	b.mu.Lock()
	b.claimGen++
	gen := b.claimGen
	b.console, b.claimed = w, true
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		if b.claimGen == gen {
			b.claimGen++
			b.console, b.claimed = nil, false
		}
		b.mu.Unlock()
	}
}

// Claimed reports whether something owns the port - so the UI can say why the
// console is quiet rather than appearing broken.
func (b *Bridge) Claimed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.claimed
}

// Type sends bytes to the node's serial input, as if someone had typed them.
//
// The caller supplies the line ending, because which one a command needs is the
// firmware's business and differs between applications.
func (b *Bridge) Type(input []byte) error {
	if b.consoleIn != nil {
		if _, err := b.consoleIn.Write(input); err != nil {
			return fmt.Errorf("firmware: %s console input: %w", b.node, err)
		}
		return nil
	}
	if !b.hasConsole {
		return fmt.Errorf(
			"firmware: %s has no console: this backend's bridge carries RF only, "+
				"so anything typed at it would be discarded without being run", b.node)
	}
	if err := b.send(kindConsoleIn, input); err != nil {
		return fmt.Errorf("firmware: console input: %w", err)
	}
	return nil
}
