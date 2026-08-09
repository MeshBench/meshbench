package companion

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// PTY exposes a node as a virtual serial device, so anything expecting a USB
// MeshCore companion — the CLI, existing flashing and config tooling, screen —
// attaches unmodified.
//
// Same length-prefixed framing as the TCP transport: a PTY is a stream, and a
// stream needs boundaries restored.
type PTY struct {
	master *os.File
	Path   string // the slave path a client opens, e.g. /dev/pts/7
	node   Node
	quit   chan struct{}
}

// OpenPTY allocates a pseudo-terminal pair and starts pumping frames.
func OpenPTY(node Node) (*PTY, error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("companion: open ptmx: %w", err)
	}
	if err := unix.IoctlSetPointerInt(int(m.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		_ = m.Close()
		return nil, fmt.Errorf("companion: unlock pty: %w", err)
	}
	n, err := unix.IoctlGetInt(int(m.Fd()), unix.TIOCGPTN)
	if err != nil {
		_ = m.Close()
		return nil, fmt.Errorf("companion: pty number: %w", err)
	}
	// Raw mode. A pty defaults to canonical line discipline, which buffers by
	// line and mangles control bytes — fatal for binary framing, and it fails as
	// a silent timeout rather than an error.
	if err := setRaw(int(m.Fd())); err != nil {
		_ = m.Close()
		return nil, fmt.Errorf("companion: raw mode: %w", err)
	}

	p := &PTY{master: m, Path: fmt.Sprintf("/dev/pts/%d", n), node: node, quit: make(chan struct{})}
	go p.pumpOut()
	go p.pumpIn()
	return p, nil
}

func (p *PTY) Close() error {
	select {
	case <-p.quit:
	default:
		close(p.quit)
	}
	return p.master.Close()
}

// pumpIn reads framed writes from whatever opened the slave and hands them to
// the node.
func (p *PTY) pumpIn() {
	var hdr [2]byte
	for {
		if _, err := io.ReadFull(p.master, hdr[:]); err != nil {
			return
		}
		n := binary.BigEndian.Uint16(hdr[:])
		if n == 0 {
			continue
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(p.master, buf); err != nil {
			return
		}
		if err := p.node.Write(buf); err != nil {
			return
		}
	}
}

// pumpOut writes the node's frames to the slave.
func (p *PTY) pumpOut() {
	frames, cancel := p.node.Subscribe()
	defer cancel()
	for {
		select {
		case <-p.quit:
			return
		case f, ok := <-frames:
			if !ok {
				return
			}
			var hdr [2]byte
			binary.BigEndian.PutUint16(hdr[:], uint16(len(f)))
			if _, err := p.master.Write(append(hdr[:], f...)); err != nil {
				return
			}
		}
	}
}

// setRaw puts the terminal into raw mode: no line discipline, no echo, no
// special-character handling. Everything a binary transport needs turned off.
func setRaw(fd int) error {
	t, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return err
	}
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	t.Oflag &^= unix.OPOST
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB
	t.Cflag |= unix.CS8
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 0
	return unix.IoctlSetTermios(fd, unix.TCSETS, t)
}
