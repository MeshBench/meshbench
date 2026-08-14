//go:build darwin

package companion

import (
	"bytes"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// The same virtual serial device, in Darwin's dialect.
//
// Linux unlocks a pty with TIOCSPTLCK and finds its name by number
// (TIOCGPTN -> /dev/pts/N). Darwin has neither: it grants and unlocks with
// TIOCPTYGRANT and TIOCPTYUNLK, and hands back the slave's *path* through
// TIOCPTYGNAME, because its ptys are named /dev/ttysNNN rather than numbered.
// Termios is the same idea under different ioctl names.

// OpenSerial creates a virtual serial device for one node.
func OpenSerial(s Serial) (*PTYLink, error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("companion: open ptmx: %w", err)
	}
	fd := m.Fd()
	if err := ioctl(fd, unix.TIOCPTYGRANT, 0); err != nil {
		_ = m.Close()
		return nil, fmt.Errorf("companion: grant pty: %w", err)
	}
	if err := ioctl(fd, unix.TIOCPTYUNLK, 0); err != nil {
		_ = m.Close()
		return nil, fmt.Errorf("companion: unlock pty: %w", err)
	}
	// 128 bytes because that is the buffer size the ioctl is specified
	// against; the answer is a NUL-terminated path inside it.
	name := make([]byte, 128)
	if err := ioctl(fd, unix.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))); err != nil {
		_ = m.Close()
		return nil, fmt.Errorf("companion: pty name: %w", err)
	}
	path := string(bytes.TrimRight(name[:bytes.IndexByte(name, 0)+1], "\x00"))

	if err := setRaw(int(fd)); err != nil {
		_ = m.Close()
		return nil, fmt.Errorf("companion: raw mode: %w", err)
	}

	l := &PTYLink{master: m, path: path, pipe: NewPipe(s)}
	l.pipe.attach(m)
	return l, nil
}

// setRaw puts a pty into raw mode. See the Linux twin for why it matters.
func setRaw(fd int) error {
	t, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return err
	}
	rawTermios(t)
	return unix.IoctlSetTermios(fd, unix.TIOCSETA, t)
}

func ioctl(fd, req, arg uintptr) error {
	if _, _, e := unix.Syscall(unix.SYS_IOCTL, fd, req, arg); e != 0 {
		return e
	}
	return nil
}
