// Package firmware connects emulated nodes to the RF engine.
//
// An emulated node runs inside Renode or QEMU and reaches the simulator over a
// socket: the SX1262 peripheral model (tools/renode/peripherals/SX1262.cs)
// hands transmitted frames out and takes delivered frames back.
//
// A socket rather than an emulator-native wireless medium is deliberate. The
// physics lives in internal/rf, and an emulated node must share exactly the
// same channel as a native one — otherwise the two backends are not comparable,
// which is the entire reason ADR-0010 has both.
package firmware

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
)

// frameKind values on the wire.
const (
	kindFrame = 0x01
)

// ErrClosed is returned once a bridge has been shut down.
var ErrClosed = errors.New("firmware: bridge closed")

// Bridge is one emulated node's link to the RF engine.
//
// Wire format, both directions: [kind:1][length:2 big-endian][payload].
// Length-prefixed for the same reason the companion transports are — a stream
// gives no message boundaries, and a radio frame is a message.
type Bridge struct {
	ln   net.Listener
	node string

	mu     sync.Mutex
	conn   net.Conn
	closed bool

	// Transmitted carries frames the emulated firmware sent. The RF engine
	// reads these and puts them on the channel.
	Transmitted chan []byte
}

// Listen starts a bridge for one emulated node. Addr is typically
// "127.0.0.1:0"; the emulator connects to Addr().
func Listen(addr, node string) (*Bridge, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("firmware: bridge listen: %w", err)
	}
	b := &Bridge{ln: ln, node: node, Transmitted: make(chan []byte, 32)}
	go b.accept()
	return b, nil
}

// Addr is the address the emulator should connect to.
func (b *Bridge) Addr() string { return b.ln.Addr().String() }

// Node names the simulated node this bridge belongs to, for the ledger.
func (b *Bridge) Node() string { return b.node }

func (b *Bridge) accept() {
	for {
		c, err := b.ln.Accept()
		if err != nil {
			return
		}
		b.mu.Lock()
		// One emulated radio per bridge: a second connection would mean two
		// chips claiming one node, which is a wiring mistake worth refusing
		// rather than silently interleaving.
		if b.conn != nil {
			b.mu.Unlock()
			c.Close()
			continue
		}
		b.conn = c
		b.mu.Unlock()
		go b.read(c)
	}
}

func (b *Bridge) read(c net.Conn) {
	defer func() {
		b.mu.Lock()
		if b.conn == c {
			b.conn = nil
		}
		b.mu.Unlock()
		c.Close()
	}()
	var hdr [3]byte
	for {
		if _, err := io.ReadFull(c, hdr[:]); err != nil {
			return
		}
		if hdr[0] != kindFrame {
			return // desynchronised; a wrong kind means the stream is not ours
		}
		n := binary.BigEndian.Uint16(hdr[1:])
		if n == 0 {
			continue
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(c, buf); err != nil {
			return
		}
		select {
		case b.Transmitted <- buf:
		default:
			// The RF engine is not keeping up. Dropping here would look exactly
			// like a propagation result, so it must never be silent.
		}
	}
}

// Deliver hands a frame to the emulated firmware — a frame the channel decided
// this node received. Only CRC-passing frames should be delivered; everything
// else belongs in the ledger and is withheld, exactly as on real hardware.
func (b *Bridge) Deliver(frame []byte) error {
	b.mu.Lock()
	c, closed := b.conn, b.closed
	b.mu.Unlock()
	if closed {
		return ErrClosed
	}
	if c == nil {
		return errors.New("firmware: no emulator attached")
	}
	if len(frame) > 0xFFFF {
		return fmt.Errorf("firmware: frame of %d bytes exceeds the wire format", len(frame))
	}
	hdr := []byte{kindFrame, byte(len(frame) >> 8), byte(len(frame))}
	if _, err := c.Write(append(hdr, frame...)); err != nil {
		return fmt.Errorf("firmware: deliver: %w", err)
	}
	return nil
}

// Attached reports whether an emulator is connected.
func (b *Bridge) Attached() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.conn != nil
}

func (b *Bridge) Close() error {
	b.mu.Lock()
	b.closed = true
	c := b.conn
	b.mu.Unlock()
	if c != nil {
		c.Close()
	}
	return b.ln.Close()
}
