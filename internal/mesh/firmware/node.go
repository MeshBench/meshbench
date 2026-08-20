package firmware

import (
	"context"
	"fmt"
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
	// Asked rather than assumed, because the two backends differ and the
	// difference is invisible from here. The native shim implements the console
	// frames. The emulated path does not: its peer is radioserver, which models
	// an SX1262 over SPI and has no UART, and the emulator's own serial is
	// opened write-only. Console input sent that way is accepted by the socket
	// and dropped, and reporting success for it is how four capability rows
	// came to describe a board's own boot advert as a transmission on command.
	HasConsole() bool
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
