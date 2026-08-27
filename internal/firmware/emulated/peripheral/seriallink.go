package peripheral

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// A SerialLink is an emulated node's serial port, both ways.
//
// The emulator publishes the port as a unix socket and we connect to it once
// it exists, which is not at once: the socket appears when the machine starts,
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

// DialSerial connects to the emulator's serial socket in the background and
// pumps everything it says into log.
func DialSerial(ctx context.Context, path string, log io.Writer) *SerialLink {
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
			if conn, err = net.Dial("unix", path); err == nil {
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
