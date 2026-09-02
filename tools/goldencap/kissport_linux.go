//go:build linux

// Opening the modem's serial port, which is termios and therefore Linux.
//
// The framing in kiss.go is portable and the port is not: TCGETS, CBAUD and
// the rest are Linux spellings, and x/sys/unix does not define them
// elsewhere. Split out so that go build ./... compiles the tree on every
// platform - it did not on Windows, which made the one cheap Windows smoke
// check unusable for the week internal/app/session was broken there.
package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func openKISS(path string) (*kissPort, error) {
	f, err := os.OpenFile(path, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, err
	}
	fd := int(f.Fd())
	t, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("termios get: %w", err)
	}
	// Raw 115200 8N1: no echo, no canonical mode, no CR mangling - a binary
	// protocol survives none of the terminal's kindnesses.
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	t.Oflag &^= unix.OPOST
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB | unix.CBAUD
	t.Cflag |= unix.CS8 | unix.CREAD | unix.CLOCAL | unix.B115200
	t.Ispeed, t.Ospeed = unix.B115200, unix.B115200
	t.Cc[unix.VMIN], t.Cc[unix.VTIME] = 0, 1 // 100 ms read granularity
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, t); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("termios set: %w", err)
	}
	return &kissPort{f: f}, nil
}

// readRaw is a raw read on the port's descriptor rather than os.File.Read:
// with VMIN=0/VTIME=1 a quiet 100 ms window returns zero bytes, which
// os.File reports as io.EOF - a serial port pausing for breath is not a
// closed file. An interrupted read is not an error either.
func (k *kissPort) readRaw(buf []byte) (int, error) {
	n, err := unix.Read(int(k.f.Fd()), buf)
	if err != nil && err != unix.EINTR {
		return n, err
	}
	return n, nil
}
