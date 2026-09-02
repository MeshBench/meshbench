package firmware

import (
	"context"
	"fmt"
	"io"
)

// A Backend is one way of running MeshCore's firmware.
//
// There are two, and ADR-0010 keeps both on purpose. Emulated runs the real
// binary on an emulated MCU — authoritative, and slow enough that a hundred of
// them will not fit in a run. Native compiles the same MeshCore sources for the
// host — fast enough to be the default, and unable to catch anything that
// depends on the target's instruction set, timing or peripherals.
//
// Neither is a fallback for the other. Native is what you run; emulated is what
// you check native against (MSIM-40), and a divergence between them is a real
// finding either way round.
type Backend interface {
	// Start launches the firmware and points it at a bridge address. It
	// returns once the process exists, not once it has connected.
	Start(ctx context.Context, bridgeAddr string) error

	// Stop terminates it. Safe to call on a backend that never started.
	Stop() error

	// Kind is "native" or "emulated". It goes in the ledger and in exports,
	// because a result that does not say which backend produced it cannot be
	// compared with one that does.
	Kind() string

	// HasConsole says whether anything on the far end of the bridge reads the
	// node's serial port.
	//
	// Asked rather than assumed, because the backends differ and the difference
	// is invisible from here. The native shim implements the console frames. An
	// emulated node's bridge peer does not - radioserver models an SX1262 over
	// SPI and has no UART - so it carries its own port instead, and whether it
	// has one depends on the emulator and on where the board's firmware put
	// Serial. Answering yes without one is how four capability rows came to
	// describe a board's own boot advert as a transmission on command: the
	// bytes are accepted by a socket and dropped.
	HasConsole() bool

	// ConsoleIn is where bytes typed at this node's serial port should go, for
	// a backend that carries its own serial channel. Nil means the console
	// rides the bridge instead, as the native shim's does.
	ConsoleIn() io.Writer
}

// A Node is a MeshCore instance plus the bridge that carries its RF.
type Node struct {
	Bridge  *Bridge
	Backend Backend
}

// Start brings up a node: bridge first, then the firmware pointed at it.
func Start(ctx context.Context, name string, b Backend) (*Node, error) {
	br, err := Listen("127.0.0.1:0", name)
	if err != nil {
		return nil, err
	}
	if err := b.Start(ctx, br.Addr()); err != nil {
		_ = br.Close()
		return nil, fmt.Errorf("firmware: start %s (%s): %w", name, b.Kind(), err)
	}
	br.hasConsole = b.HasConsole()
	br.consoleIn = b.ConsoleIn()
	// A backend with its own serial port has to be asked to copy it here.
	// Input already went straight to the port and output went straight to a
	// file, so an emulated node could be typed at and never answered: the
	// console pane, the companion client and meshcore-cli all read the bridge,
	// and nothing had ever written to it for these backends.
	if t, ok := b.(interface{ TeeConsole(io.Writer) }); ok {
		t.TeeConsole(br.ConsoleSink())
	}
	return &Node{Bridge: br, Backend: b}, nil
}

// Close shuts a node down, bridge first.
//
// Dropping the socket is how a node is told to stop: its read loop ends, it
// reports its final counters and exits. Killing the process first instead skips
// all of that — and those counters are usually the only evidence available
// about what a misbehaving node actually did.
func (n *Node) Close() error {
	err := n.Bridge.Close()
	if serr := n.Backend.Stop(); err == nil {
		err = serr
	}
	return err
}
