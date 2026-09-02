package peripheral

import (
	"context"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// A SerialLink is an emulated node's serial port, both ways.
//
// The emulator publishes the port as a socket and we connect to it once it
// exists, which is not at once: the socket appears when the machine starts,
// and the caller may ask to type at the node before then. Writes therefore
// wait for the connection rather than failing on a race the caller cannot see.
//
// Everything the node prints is copied on to the console log, because that file
// has always carried the whole boot chain - ROM through application - and is
// where an emulated node's failures are read.
type SerialLink struct {
	ready chan struct{}

	mu   sync.Mutex
	conn net.Conn
	err  error
}

// serialNetwork is how a serial address is reached: a path is a unix socket, and
// anything else is a host and port.
//
// The two emulators publish the port differently and neither can be talked into
// the other's form. QEMU is given a socket file, which costs no port and cannot
// collide; Renode's server terminal only ever listens on TCP. Deciding here
// rather than at each call site means one rule, and a caller that holds an
// address does not also have to remember which emulator produced it.
func serialNetwork(addr string) string {
	// A separator, rather than "is it absolute": a node's socket is always
	// joined onto its working directory, and a host and port has no separator
	// in it on either platform.
	if strings.ContainsRune(addr, filepath.Separator) {
		return "unix"
	}
	return "tcp"
}

// DialSerial connects to the emulator's serial socket in the background and
// pumps everything it says into log.
func DialSerial(ctx context.Context, addr string, log io.Writer) *SerialLink {
	network := serialNetwork(addr)
	s := &SerialLink{ready: make(chan struct{})}
	go func() {
		var conn net.Conn
		var err error
		// The emulator creates the socket as it starts. Retried rather than
		// waited on by a fixed sleep, because how long that takes is a
		// property of the machine this is running on.
		for i := 0; i < 400; i++ {
			if ctx.Err() != nil {
				err = ctx.Err()
				break
			}
			if conn, err = net.Dial(network, addr); err == nil {
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		s.mu.Lock()
		s.conn, s.err = conn, err
		s.mu.Unlock()
		close(s.ready)
		if conn == nil {
			return
		}
		_, _ = io.Copy(log, conn)
	}()
	return s
}

// Write types bytes at the node, waiting for the port to exist first.
func (s *SerialLink) Write(p []byte) (int, error) {
	select {
	case <-s.ready:
	case <-time.After(10 * time.Second):
		return 0, fmt.Errorf("firmware: the node's serial port never appeared")
	}
	s.mu.Lock()
	conn, err := s.conn, s.err
	s.mu.Unlock()
	if conn == nil {
		return 0, fmt.Errorf("firmware: the node's serial port did not open: %w", err)
	}
	return conn.Write(p)
}

func (s *SerialLink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return nil
	}
	return s.conn.Close()
}
