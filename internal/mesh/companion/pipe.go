package companion

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

// Serial is a node's serial port, as the simulator sees it.
//
// Bytes in both directions, unframed. MeshCore's own framing — '>' or '<' then
// a 16-bit little-endian length then the payload — lives inside these bytes and
// is not this layer's business: a transport that re-frames what is already
// framed produces a stream no client can parse, which is exactly what the first
// attempt did.
type Serial interface {
	// Write sends bytes to the node's serial input.
	Write([]byte) error
	// Attach directs the node's serial output at w, replacing any previous
	// reader. One reader, because a UART has one wire.
	Attach(w io.Writer)
	// Detach stops directing output anywhere.
	Detach()
}

// Pipe is a byte-transparent link between a client and a node's serial port.
//
// The whole design: whatever the client sends reaches the firmware's UART
// unaltered, and whatever the firmware prints reaches the client unaltered. A
// phone app or meshcore-cli then cannot tell the difference between this and a
// USB cable, because at the byte level there is none.
type Pipe struct {
	serial Serial

	// OnStall is called once when an attached client first fails to take what
	// the firmware printed. See the note below.
	OnStall func()

	mu      sync.Mutex
	client  io.WriteCloser
	closed  bool
	stalled bool
}

// writeWindow is how long the firmware's output waits for a client to take it.
//
// Generous by the standards of a socket and instant by the standards of a
// simulation: a client that is running keeps up easily, and one that has
// stopped costs a tenth of a second per write instead of everything.
const writeWindow = 100 * time.Millisecond

// deadliner is what a transport offers if it can bound a write. Both of the
// ones here can - a TCP conn and a pty are each backed by a file descriptor.
type deadliner interface{ SetWriteDeadline(time.Time) error }

// NewPipe wires a serial port to whatever attaches next.
func NewPipe(s Serial) *Pipe { return &Pipe{serial: s} }

// OnStall is called once when an attached client first fails to take what the
// firmware printed, so the interface can say so. Silence is the one thing this
// failure cannot be allowed to be.
//
// Set it before attaching anything.

// Write implements io.Writer for the firmware's output, forwarding to the
// attached client.
func (p *Pipe) Write(b []byte) (int, error) {
	p.mu.Lock()
	c := p.client
	p.mu.Unlock()
	if c == nil {
		// Nobody is listening. Dropped rather than buffered: a client that
		// attaches later wants what happens next, not a replay of everything
		// since the node booted.
		return len(b), nil
	}
	// Bounded, because an attached client that has stopped reading is not the
	// same as one that has gone away and used to be worse. The socket's send
	// buffer fills, this write blocks, and the firmware behind it stops - so
	// the whole simulation waits on somebody's paused debugger. Sixty seconds
	// of simulated time did not finish in two and a half minutes of real time.
	//
	// A real radio does not wait either. It puts bytes on a wire whether or not
	// anything is listening.
	if d, ok := c.(deadliner); ok {
		_ = d.SetWriteDeadline(time.Now().Add(writeWindow))
	}
	_, err := c.Write(b)
	if d, ok := c.(deadliner); ok {
		_ = d.SetWriteDeadline(time.Time{})
	}
	switch {
	case err == nil:
		p.clearStall()
	case errors.Is(err, os.ErrDeadlineExceeded):
		// Kept, not dropped: a client that is merely slow should not lose its
		// session over one burst. The bytes are gone, which is what a UART does
		// to a listener that is not there, and it is said once rather than on
		// every write - a stall produces thousands.
		p.noteStall()
	default:
		// The client went away mid-write. Drop it and carry on: the node's
		// serial port accepted these bytes, and a firmware that stalled because
		// somebody closed a phone app would be a simulation of nothing.
		p.dropClient(c)
	}
	return len(b), nil
}

// noteStall reports the first write a client failed to take, and says nothing
// on the thousands that follow.
func (p *Pipe) noteStall() {
	p.mu.Lock()
	first := !p.stalled
	p.stalled = true
	p.mu.Unlock()
	if first && p.OnStall != nil {
		p.OnStall()
	}
}

func (p *Pipe) clearStall() {
	p.mu.Lock()
	p.stalled = false
	p.mu.Unlock()
}

// Stalled reports whether the attached client failed to take the last thing
// written to it.
//
// Worth asking, because the symptom is silence: a client that has stopped
// reading looks exactly like a mesh with nothing to say.
func (p *Pipe) Stalled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stalled
}

// dropClient detaches a client that is no longer readable or writable.
func (p *Pipe) dropClient(c io.WriteCloser) {
	p.mu.Lock()
	if p.client == c {
		p.client = nil
		p.serial.Detach()
	}
	p.mu.Unlock()
	_ = c.Close()
}

// attach takes over the serial port for one client.
func (p *Pipe) attach(rw io.ReadWriteCloser) {
	p.mu.Lock()
	if p.client != nil {
		_ = p.client.Close() // one client at a time, as with one USB port
	}
	p.client = rw
	p.mu.Unlock()
	p.serial.Attach(p)

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := rw.Read(buf)
			if n > 0 {
				if err := p.serial.Write(buf[:n]); err != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		p.dropClient(rw)
	}()
}

// Attached reports whether a client currently holds the port.
//
// This is what the speed pin reads: ADR-0008 pins simulated time to wall time
// while a real client is *attached*, not merely while a listener exists — a
// TCP port with nobody connected constrains nothing.
func (p *Pipe) Attached() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.client != nil
}

// Close drops the current client.
func (p *Pipe) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	if p.client != nil {
		_ = p.client.Close()
		p.client = nil
	}
	p.serial.Detach()
	return nil
}

// TCPLink serves one node's serial port over TCP.
//
// The transport MeshCore's own WiFi companions use: the same framed stream, on
// a socket instead of a wire.
type TCPLink struct {
	ln   net.Listener
	pipe *Pipe
}

// ListenTCP starts a TCP server for one node's serial port.
func ListenTCP(addr string, s Serial) (*TCPLink, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("companion: listen: %w", err)
	}
	l := &TCPLink{ln: ln, pipe: NewPipe(s)}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			l.pipe.attach(c)
		}
	}()
	return l, nil
}

func (l *TCPLink) Addr() string { return l.ln.Addr().String() }

// Attached reports whether a client is connected right now.
func (l *TCPLink) Attached() bool { return l.pipe.Attached() }

// Stalled reports whether the attached client has stopped taking output.
func (l *TCPLink) Stalled() bool { return l.pipe.Stalled() }

// OnStall is called once when a client first fails to take what the firmware
// printed. Set it before anything attaches.
func (l *TCPLink) OnStall(f func()) { l.pipe.OnStall = f }

func (l *TCPLink) Close() error {
	_ = l.pipe.Close()
	return l.ln.Close()
}

// PTYLink presents a node's serial port as a virtual serial device.
//
// A real character device the operating system hands out, so software that
// wants a serial port — meshcore-cli, a phone app over USB tethering, anything
// that opens /dev/pts/* — attaches without knowing it is talking to a
// simulation. TCP is convenient; this is the one that makes a client believe it
// is plugged in.
type PTYLink struct {
	master *os.File
	path   string
	pipe   *Pipe
}

// Path is the device to point client software at.
func (l *PTYLink) Path() string { return l.path }

// Attached reports whether the pty's client side is held. The master is held
// by the link itself, so this is true from creation — a pty has no accept
// event, and pretending otherwise would leave the pin off while a client is
// mid-conversation.
func (l *PTYLink) Attached() bool { return l.pipe.Attached() }

// Stalled reports whether the attached client has stopped taking output.
func (l *PTYLink) Stalled() bool { return l.pipe.Stalled() }

// OnStall is called once when a client first fails to take what the firmware
// printed. Set it before anything attaches.
func (l *PTYLink) OnStall(f func()) { l.pipe.OnStall = f }

func (l *PTYLink) Close() error {
	_ = l.pipe.Close()
	return l.master.Close()
}
