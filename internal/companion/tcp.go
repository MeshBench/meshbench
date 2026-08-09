// Package companion exposes each simulated node's companion interface so real
// clients — MeshCIM, the official apps, the CLI — can drive it.
//
// No management traffic ever crosses the simulated air (ADR-0014). These
// transports are out of band by construction, which is what keeps commanding a
// node from perturbing the measurement.
package companion

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
)

// Frame is one companion-protocol message. Framing matches what the BLE
// characteristic delivers: a length-prefixed payload, so a stream transport
// reproduces the message boundaries a packet transport gives for free.
type Frame []byte

// Node is what a transport needs from the simulator: somewhere to send frames
// the client wrote, and a way to be told about frames going the other way.
type Node interface {
	// Write is called when a client sends a frame to this node.
	Write(Frame) error
	// Subscribe returns a channel of frames from the node to the client, and a
	// cancel function. One subscriber per connection.
	Subscribe() (<-chan Frame, func())
}

// Server serves one node over TCP.
//
// Bound to localhost by default: a simulator that opens forty ports on
// 0.0.0.0 is a surprise nobody asked for. Exposing on the LAN is deliberate,
// because a phone running MeshCIM cannot reach loopback.
type Server struct {
	node     Node
	ln       net.Listener
	onAttach func(bool)

	mu    sync.Mutex
	conns int
}

// Listen starts a TCP server for one node. addr is typically "127.0.0.1:0".
//
// onAttach is called with true when the first client connects and false when the
// last disconnects — the hook the simulator uses to pin the clock to 1x, which
// ADR-0008 requires be visible rather than a hidden coupling.
func Listen(addr string, node Node, onAttach func(bool)) (*Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("companion: listen: %w", err)
	}
	s := &Server{node: node, ln: ln, onAttach: onAttach}
	go s.accept()
	return s, nil
}

// Addr is the address actually bound, which matters when port 0 was requested.
func (s *Server) Addr() string { return s.ln.Addr().String() }

func (s *Server) Close() error { return s.ln.Close() }

func (s *Server) accept() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return // listener closed
		}
		go s.serve(c)
	}
}

func (s *Server) serve(c net.Conn) {
	defer c.Close()
	s.attached(1)
	defer s.attached(-1)

	frames, cancel := s.node.Subscribe()
	defer cancel()

	// Node -> client. quit is closed when the read side ends, so the writer
	// never outlives the connection — without it, serve would block forever
	// waiting on a frames channel nobody closes, and the detach event that
	// unpins the clock would never fire.
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-quit:
				return
			case f, ok := <-frames:
				if !ok {
					return
				}
				var hdr [2]byte
				binary.BigEndian.PutUint16(hdr[:], uint16(len(f)))
				if _, err := c.Write(hdr[:]); err != nil {
					return
				}
				if _, err := c.Write(f); err != nil {
					return
				}
			}
		}
	}()

	// Client -> node.
	var hdr [2]byte
	for {
		if _, err := io.ReadFull(c, hdr[:]); err != nil {
			break
		}
		n := binary.BigEndian.Uint16(hdr[:])
		if n == 0 {
			continue
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(c, buf); err != nil {
			break
		}
		if err := s.node.Write(buf); err != nil {
			break
		}
	}
	close(quit)
}

func (s *Server) attached(delta int) {
	s.mu.Lock()
	before := s.conns
	s.conns += delta
	after := s.conns
	s.mu.Unlock()
	if s.onAttach == nil {
		return
	}
	// Fire only on the transitions that matter: first attach and last detach.
	if before == 0 && after > 0 {
		s.onAttach(true)
	} else if before > 0 && after == 0 {
		s.onAttach(false)
	}
}
