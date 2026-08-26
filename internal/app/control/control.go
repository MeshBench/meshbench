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
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// Protocol is the wire version, bumped only when a change breaks a client
// written against the previous number.
//
// Here rather than beside session.hello, which is what reports it: this is the
// package that defines the wire, and a client should be able to know what it
// speaks without importing a simulator.
//
// Adding a verb does not break anybody, and neither does adding a field to a
// result: a client reads the fields it knows. What moves this is a verb
// changing what it means, a field changing type, or the framing changing.
const Protocol = 1

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
	// Code classifies the failure, because the message is prose and prose is
	// not a thing to match on. Telling "no node named X" from "the workbench
	// is closing" meant comparing strings written in two different files by
	// two different authors.
	//
	// The message stays exactly as the verb wrote it. The verbs in this tree
	// write good prose - "no node is running firmware, so there is nothing to
	// send to" - and a client that replaced that with a code would be making
	// the experience worse.
	Code string `json:"code,omitempty"`
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
	addr Address
	// rendezvous is the file naming a TCP listener, empty for a unix socket.
	rendezvous string

	mu      sync.Mutex
	queue   []job
	handler Handler
	closed  bool
	// subs are the connections that asked to be told what changed. Guarded by
	// mu, like the queue. clock is time.Now unless a test pins it.
	subs  map[*subscriber]struct{}
	clock func() time.Time
}

type job struct {
	req   Request
	reply chan Response
}

// Listen starts the control socket where this operating system answers.
func Listen(h Handler) (*Server, error) { return ListenAt("", h) }

// ListenAt starts it where the caller asked, and refuses to take a live
// address from whoever already holds it.
//
// Removing whatever was there was aimed at the socket a crashed run leaves
// behind, and it hit a live workbench just as squarely: the second process
// unlinked the first one's socket and bound its own. The first kept running
// with a listener nothing could reach, everything already connected to it
// carried on working, and neither end had any way to notice. So the address is
// dialled first, and only one that answers nothing is cleared away - which is
// the crashed-run case, and only that case.
func ListenAt(want string, h Handler) (*Server, error) {
	addr, err := Resolve(want)
	if err != nil {
		return nil, err
	}
	if live(addr) {
		return nil, fmt.Errorf(
			"control: %s is already answering - another workbench holds it. "+
				"Choose another with -control-socket or %s", addr, SocketEnv)
	}
	s := &Server{handler: h}
	switch addr.Kind {
	case Unix:
		_ = os.Remove(addr.Addr)
		ln, err := net.Listen("unix", addr.Addr)
		if err != nil {
			return nil, fmt.Errorf("control: listen: %w", err)
		}
		// The filesystem is the access control here, so it is set rather than
		// inherited from whatever umask happens to be in force.
		if err := os.Chmod(addr.Addr, 0o600); err != nil {
			_ = ln.Close()
			return nil, fmt.Errorf("control: securing %s: %w", addr.Addr, err)
		}
		s.ln, s.addr = ln, addr
	case TCP:
		ln, err := net.Listen("tcp", addr.Addr)
		if err != nil {
			return nil, fmt.Errorf("control: listen: %w", err)
		}
		// The port is only knowable once it is bound, so the address is
		// completed here and then written down for a client to find.
		addr.Addr = ln.Addr().String()
		if addr.Token, err = newToken(); err != nil {
			_ = ln.Close()
			return nil, err
		}
		if s.rendezvous, err = writeRendezvous(addr.Addr, addr.Token); err != nil {
			_ = ln.Close()
			return nil, err
		}
		s.ln, s.addr = ln, addr
	}
	go s.accept()
	return s, nil
}

// live reports whether something is already answering there.
//
// A dial rather than a stat: a socket file existing says nothing about whether
// anybody is behind it, and that difference is the whole of this check. For
// TCP it is the port that is probed and not the rendezvous file, for the same
// reason - a file left by a crashed run names a port nobody holds.
func live(addr Address) bool {
	network, target := "unix", addr.Addr
	if addr.Kind == TCP {
		network = "tcp"
		// An ephemeral port cannot already be held, because it has not been
		// chosen yet. What might be is whatever the rendezvous file still names.
		if strings.HasSuffix(target, ":0") {
			r, err := readRendezvous()
			if err != nil {
				return false
			}
			target = r.Address
		}
	}
	// No context on purpose. This is a 250 ms probe of something on this
	// machine, answered or not, and it happens once before the listener exists -
	// so there is no caller yet whose cancellation it could honour and nothing
	// that could outlast the timeout. Threading a context in would put one on
	// ListenAt that controlled nothing but this line, which reads as a lifetime
	// and is not one.
	//nolint:noctx // bounded local probe, before there is a server to cancel
	c, err := net.DialTimeout(network, target, 250*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// Path is where this server answers, as somebody would type it back in: a
// socket path, or tcp:host:port.
func (s *Server) Path() string { return s.addr.String() }

// Address is the same, in full - including the token a TCP client needs.
func (s *Server) Address() Address { return s.addr }

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
		j.reply <- Response{ID: j.req.ID, Error: "workbench closing",
			Code: string(Closing)}
	}
	err := s.ln.Close()
	if s.addr.Kind == Unix {
		_ = os.Remove(s.addr.Addr)
	}
	if s.rendezvous != "" {
		// Removed, so the next start does not probe a port nobody holds and a
		// client does not connect to whatever has since taken it.
		_ = os.Remove(s.rendezvous)
	}
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
	if s.addr.Kind == TCP && !s.authorised(c, dec, enc) {
		return
	}

	// One writer, so replies and notifications never interleave a half-frame on
	// the wire. The reader still takes one request at a time; notifications are
	// pushed onto out from Notify, on the store's goroutine, and the writer
	// serialises the two. A buffered channel means a slow client stalls its own
	// notifications (dropped in Notify) rather than the store.
	out := make(chan any, 64)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case msg := <-out:
				if enc.Encode(msg) != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()

	var sub *subscriber
	defer func() {
		if sub != nil {
			s.unsubscribe(sub)
		}
		close(done)
	}()

	for {
		var req Request
		if err := dec.Decode(&req); err != nil {
			return
		}

		// session.subscribe is a connection-level concern, not a world verb: it
		// registers this connection's interest and replies, without a trip
		// through the handler.
		//
		// The acknowledgement is queued to the writer before the subscription
		// is registered, and that order matters: register first and a publish
		// in the gap would push a notification into the same queue ahead of the
		// ack, and the client's subscribe Call - which decodes exactly one
		// reply - would read that notification as its answer. Queued first, the
		// ack is always the first frame the writer sends.
		if req.Method == SubscribeMethod {
			topics := parseTopics(req.Params)
			out <- Response{ID: req.ID, Result: map[string]any{"subscribed": topics}}
			if sub == nil {
				sub = s.subscribe(out, topics)
			} else {
				s.setTopics(sub, topics)
			}
			continue
		}

		reply := make(chan Response, 1)
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			out <- Response{ID: req.ID, Error: "workbench closing", Code: string(Closing)}
			return
		}
		s.queue = append(s.queue, job{req: req, reply: reply})
		s.mu.Unlock()

		out <- <-reply
	}
}

// authorised reads the first line of a TCP connection and checks its token.
//
// Only for TCP. A unix socket is protected by its permissions and always has
// been, and asking a token of it as well would break every script written
// against the socket so far for no gain.
//
// The refusal says what is wrong and closes. Not a timing-safe comparison:
// the token is 128 bits of randomness in a 0600 file on the same machine, and
// an attacker able to time this loop is an attacker who could read the file.
func (s *Server) authorised(c net.Conn, dec *json.Decoder, enc *json.Encoder) bool {
	// A deadline, because a connection that opens and says nothing would
	// otherwise hold a goroutine for as long as it liked.
	_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
	var h hello
	if err := dec.Decode(&h); err != nil {
		return false
	}
	if h.Token != s.addr.Token {
		_ = enc.Encode(Response{
			Error: "this connection did not present the token from " +
				"the workbench's address file",
			Code: string(Unauthorised),
		})
		return false
	}
	// Cleared: a driven session is idle for minutes at a time between verbs.
	_ = c.SetReadDeadline(time.Time{})
	return true
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
			resp.Error, resp.Code = err.Error(), string(CodeOf(err))
		} else {
			resp.Result = result
		}
		j.reply <- resp
	}
}

// Client talks to a running workbench.
type Client struct {
	conn net.Conn
	addr Address
	enc  *json.Encoder
	dec  *json.Decoder
	mu   sync.Mutex
	next int
}

// Dial connects to the workbench where this operating system answers.
func Dial() (*Client, error) { return DialAt("") }

// DialAt connects to a chosen address.
//
// A TCP address is found through the rendezvous file rather than guessed at,
// because the port is ephemeral - and the token comes from the same file, so
// a client that can read it is a client entitled to drive the session.
func DialAt(want string) (*Client, error) {
	addr, err := Resolve(want)
	if err != nil {
		return nil, err
	}
	return dialAddr(addr)
}

// dialAddr opens one connection to an already-resolved address, doing the TCP
// token handshake if it needs to. Split out so a subscription can open a second
// connection to the same place - token and all - without re-resolving.
func dialAddr(addr Address) (*Client, error) {
	network := "unix"
	if addr.Kind == TCP {
		network = "tcp"
		if strings.HasSuffix(addr.Addr, ":0") || addr.Token == "" {
			r, err := readRendezvous()
			if err != nil {
				return nil, err
			}
			addr.Addr, addr.Token = r.Address, r.Token
		}
	}
	c, err := net.Dial(network, addr.Addr) //nolint:noctx // see live()
	if err != nil {
		return nil, fmt.Errorf("control: no workbench is listening at %s: %w",
			addr, err)
	}
	cl := &Client{conn: c, addr: addr, enc: json.NewEncoder(c),
		dec: json.NewDecoder(bufio.NewReader(c))}
	if addr.Kind == TCP {
		// The token first, before anything else on the wire. A loopback port
		// is reachable by any local process, so this is what stands in for the
		// permissions a unix socket would have had.
		if err := cl.enc.Encode(hello{Token: addr.Token}); err != nil {
			_ = c.Close()
			return nil, err
		}
	}
	return cl, nil
}

// hello is the first line of a TCP connection, and the only thing sent before
// the token has been accepted.
type hello struct {
	Token string `json:"token"`
}

// Path is where this client is connected.
func (c *Client) Path() string { return c.addr.String() }

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
		// The code travels with the message rather than replacing it, so
		// errors.As gets the classification and a person still gets the
		// sentence the verb wrote.
		return nil, &Coded{Code: Code(resp.Code), Err: errors.New(resp.Error)}
	}
	b, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, err
	}
	return b, nil
}
