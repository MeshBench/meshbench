//go:build linux

package companion

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// OpenSerial creates a virtual serial device for one node.
func OpenSerial(s Serial) (*PTYLink, error) {
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
	// line and mangles control bytes — fatal for binary framing, and it fails
	// as a silent timeout rather than as an error.
	if err := setRaw(int(m.Fd())); err != nil {
		_ = m.Close()
		return nil, fmt.Errorf("companion: raw mode: %w", err)
	}

	l := &PTYLink{master: m, path: fmt.Sprintf("/dev/pts/%d", n), pipe: NewPipe(s)}
	l.pipe.attach(m)
	return l, nil
}

// setRaw puts a pty into raw mode.
//
// A pty defaults to canonical line discipline: it buffers by line, translates
// CR and LF, and eats control bytes. MeshCore's framing is binary and contains
// all of those, so without this a client sees a stream that is subtly wrong —
// and it fails as a timeout waiting for a frame rather than as an error.
func setRaw(fd int) error {
	t, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return err
	}
	rawTermios(t)
	return unix.IoctlSetTermios(fd, unix.TCSETS, t)
}
