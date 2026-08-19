// Package control lets another process drive a running workbench.
//
// A unix socket that exists only while the workbench does. Not a service: there
// is nothing to deploy, nothing listens when the application is closed, and the
// socket lives in the user's runtime directory rather than on a port.
//
// The point is an assistant manipulating the session the operator is looking
// at — placing nodes, starting firmware, typing at a console — rather than
// answering questions from a second engine that shares nothing with what is on
// screen.
package control

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
)

// Request is one command.
type Request struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response is its answer.
type Response struct {
	ID     int    `json:"id"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Handler runs one command and returns something JSON-encodable.
//
// Called on the frame thread, never on the connection's goroutine. The UI is
// single-threaded and an external write racing a frame is a crash with an
// assistant's fingerprints on it — so the socket queues work and the frame loop
// performs it.
type Handler func(method string, params json.RawMessage) (any, error)

// Server accepts control connections.
type Server struct {
	ln   net.Listener
	path string

	mu      sync.Mutex
	queue   []job
	handler Handler
	closed  bool
}

type job struct {
	req   Request
	reply chan Response
}

// SocketPath is where the workbench listens.
//
// Per user, in the runtime directory, so two accounts on one machine do not
// fight over it and nothing survives a reboot.
func SocketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "meshcoresim.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("meshcoresim-%d.sock", os.Getuid()))
}

// Listen starts the control socket.
func Listen(h Handler) (*Server, error) {
	path := SocketPath()
	// A socket left by a crashed run would refuse the bind. Removing it is safe
	// because a live workbench holds it open, and a dead one has no claim.
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("control: listen: %w", err)
	}
	s := &Server{ln: ln, path: path, handler: h}
	go s.accept()
	return s, nil
}

// Path is the socket's address.
func (s *Server) Path() string { return s.path }

// Close stops listening and removes the socket.
func (s *Server) Close() error {
	s.mu.Lock()
	s.closed = true
	pending := s.queue
	s.queue = nil
	s.mu.Unlock()
	// Anything queued is answered rather than left hanging: a client blocked on
	// a reply that will never come is worse than an error.
	for _, j := range pending {
		j.reply <- Response{ID: j.req.ID, Error: "workbench closing"}
	}
	err := s.ln.Close()
	_ = os.Remove(s.path)
	return err
}

func (s *Server) accept() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.serve(c)
	}
}

func (s *Server) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	dec := json.NewDecoder(bufio.NewReader(c))
	enc := json.NewEncoder(c)
	for {
		var req Request
		if err := dec.Decode(&req); err != nil {
			return
		}
		reply := make(chan Response, 1)

		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			_ = enc.Encode(Response{ID: req.ID, Error: "workbench closing"})
			return
		}
		s.queue = append(s.queue, job{req: req, reply: reply})
		s.mu.Unlock()

		if err := enc.Encode(<-reply); err != nil {
			return
		}
	}
}

// Pump runs any queued commands. Called once per frame, from the frame thread.
//
// The whole thread-safety story in one function: commands arrive on connection
// goroutines, wait in a queue, and are executed here — where touching the UI is
// legal.
func (s *Server) Pump() {
	s.mu.Lock()
	pending := s.queue
	s.queue = nil
	h := s.handler
	s.mu.Unlock()

	for _, j := range pending {
		result, err := h(j.req.Method, j.req.Params)
		resp := Response{ID: j.req.ID}
		if err != nil {
			resp.Error = err.Error()
		} else {
			resp.Result = result
		}
		j.reply <- resp
	}
}

// Client talks to a running workbench.
type Client struct {
	conn net.Conn
	enc  *json.Encoder
	dec  *json.Decoder
	mu   sync.Mutex
	next int
}

// Dial connects to the workbench, if one is running.
func Dial() (*Client, error) {
	c, err := net.Dial("unix", SocketPath())
	if err != nil {
		return nil, fmt.Errorf("control: no workbench is listening at %s: %w", SocketPath(), err)
	}
	return &Client{conn: c, enc: json.NewEncoder(c), dec: json.NewDecoder(bufio.NewReader(c))}, nil
}

func (c *Client) Close() error { return c.conn.Close() }

// Call runs one command and returns the raw JSON result.
func (c *Client) Call(method string, params any) (json.RawMessage, error) {
	var raw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next++
	if err := c.enc.Encode(Request{ID: c.next, Method: method, Params: raw}); err != nil {
		return nil, err
	}
	var resp Response
	if err := c.dec.Decode(&resp); err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("%s", resp.Error)
	}
	b, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, err
	}
	return b, nil
}
