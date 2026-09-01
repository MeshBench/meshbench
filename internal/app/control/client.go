package control

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// Client talks to a running workbench.
type Client struct {
	conn net.Conn
	addr Address
	enc  *json.Encoder
	dec  *json.Decoder
	mu   sync.Mutex
	next int
	// broken is set once a call ends by its context rather than by a reply.
	// After that the decoder's place in the stream cannot be trusted: the
	// answer to the call that timed out may still arrive later, and a second
	// call reading it would take somebody else's reply as its own. Once set,
	// every further call fails fast with this instead of risking that.
	broken error
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

// DialAddress connects to an address already in hand, token and all, rather
// than to one that still has to be resolved.
//
// This is how a row from Sessions is connected to. Resolving its address
// again would find the token in the per-user rendezvous file, which two TCP
// workbenches share and the second overwrites - so the second session would
// be dialled with the first one's token, or the first with the second's.
func DialAddress(addr Address) (*Client, error) { return dialAddr(addr) }

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
		if err := cl.enc.Encode(hello{Token: addr.Token, Protocol: Protocol}); err != nil {
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
	// Protocol is the wire version this client speaks, checked once the token
	// is accepted. A unix socket has no line of its own to put it on, so there
	// it travels on the first request instead.
	Protocol int `json:"protocol,omitempty"`
}

// Path is where this client is connected.
func (c *Client) Path() string { return c.addr.String() }

func (c *Client) Close() error { return c.conn.Close() }

// Call runs one command and returns the raw JSON result, with no deadline of
// its own - a hang here waits as long as the workbench takes, which is what
// every caller before CallContext existed was already written to expect.
//
// Deliberately not a wrapper over CallContext(context.Background(), ...):
// that would give every existing caller's own context, sitting unused one
// frame up, a reason to be flagged as ignored, for a call that was never
// meant to carry one. Call and CallContext share the exchange itself through
// exchange, and only CallContext deals in context.Context at all.
func (c *Client) Call(method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.broken != nil {
		return nil, c.broken
	}
	return c.exchange(method, params)
}

// CallContext runs one command and returns its result, and gives up when ctx
// does.
//
// A blocking net.Conn cannot be interrupted by a select on ctx.Done(): the
// only thing that unblocks a read in progress is forcing the connection's own
// deadline into the past, so that is what the goroutine below does the moment
// ctx ends, and nothing else. It matters that replies come only from Pump, on
// the frame thread - anything that stalls the frame loop stalls every call on
// every client, which is exactly the hang this exists to bound.
//
// c.mu is held for the whole exchange, as it always was, so one call in
// flight still blocks the next one on the same Client. A caller that cannot
// afford that opens a second Client rather than waits for this one to learn
// to share.
func (c *Client) CallContext(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.broken != nil {
		return nil, c.broken
	}

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			_ = c.conn.SetDeadline(time.Now())
		case <-stop:
		}
	}()

	raw, err := c.exchange(method, params)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			// The decoder's place in the stream can no longer be trusted:
			// the answer to this call may still arrive later, and a second
			// call reading it would take somebody else's reply as its own.
			// Marked rather than silently risked, so every call after this
			// one fails fast instead.
			c.broken = fmt.Errorf(
				"control: the call was cancelled and this connection can no longer be trusted: %w", cerr)
			return nil, c.broken
		}
		return nil, err
	}
	return raw, nil
}

// exchange writes one request and reads its reply. Shared by Call and
// CallContext, neither of which it knows about: a deadline, if any, is
// already on the connection by the time this runs.
func (c *Client) exchange(method string, params any) (json.RawMessage, error) {
	var raw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		raw = b
	}

	c.next++
	req := Request{ID: c.next, Method: method, Params: raw}
	if c.next == 1 {
		// Declared on the frame this client was already sending, so a version
		// the workbench cannot speak is refused before any verb runs and
		// without a round trip of its own. Only the first: the answer cannot
		// change while the connection is open.
		req.Protocol = Protocol
	}
	if err := c.enc.Encode(req); err != nil {
		return nil, err
	}
	var resp Response
	if err := c.dec.Decode(&resp); err != nil {
		return nil, err
	}
	// Cleared, not left set: a call that finished inside its deadline leaves
	// the connection healthy, and the next call - which may carry no
	// deadline at all - should not inherit one it never asked for.
	_ = c.conn.SetDeadline(time.Time{})

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
