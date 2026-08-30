package control

import (
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The hello line gets a tight budget because nobody has proven anything yet,
// and an ordinary request gets a generous but finite one because a verb that
// legitimately carries bulk data reads it itself, off the workbench's own
// goroutine, rather than through a request's params.

// A peer that sends more than the hello budget before it has presented a
// token is told exactly why, closed, and does not take the socket down for
// anybody else.
func TestAnOversizedHelloIsRefusedAndTheServerStaysUp(t *testing.T) {
	srv := tcpServer(t)

	raw, err := net.DialTimeout("tcp", srv.Address().Addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()

	// What an unauthenticated peer forcing a huge allocation looks like on
	// the wire, before anybody has checked whether it is allowed to send
	// anything at all.
	oversized := hello{Token: strings.Repeat("a", 2*int(helloFrameLimit))}
	if err := json.NewEncoder(raw).Encode(oversized); err != nil {
		t.Fatal(err)
	}

	var resp Response
	_ = raw.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := json.NewDecoder(raw).Decode(&resp); err != nil {
		t.Fatalf("the refusal was not readable: %v", err)
	}
	if resp.Code != string(BadParams) {
		t.Fatalf("code %q, want %q (%s)", resp.Code, BadParams, resp.Error)
	}

	// Closed, not merely refused this once.
	one := make([]byte, 1)
	_ = raw.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := raw.Read(one); err == nil {
		t.Fatal("the oversized connection was not closed")
	}

	// And the socket is still answering everybody else.
	c, err := DialAt("tcp")
	if err != nil {
		t.Fatalf("the server stopped answering after an oversized peer: %v", err)
	}
	defer func() { _ = c.Close() }()
	if _, err := c.Call("who", nil); err != nil {
		t.Fatalf("call after an oversized peer: %v", err)
	}
}

// A frame past the request budget, sent once a connection is trusted, is
// refused the same way rather than left to a confusing decode failure.
func TestAnOversizedRequestIsRefusedPostAuth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.sock")
	serveAt(t, path)

	raw, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()

	type bigParams struct {
		Notes string `json:"notes"`
	}
	b, err := json.Marshal(bigParams{Notes: strings.Repeat("x", 2*requestFrameLimit)})
	if err != nil {
		t.Fatal(err)
	}
	req := Request{ID: 1, Method: "who", Params: b}
	if err := json.NewEncoder(raw).Encode(req); err != nil {
		// A write this large can itself fail if the server closes the
		// connection mid-send; that is an acceptable way for this test to
		// observe the refusal too.
		return
	}

	var resp Response
	_ = raw.SetReadDeadline(time.Now().Add(10 * time.Second))
	if err := json.NewDecoder(raw).Decode(&resp); err != nil {
		// Also acceptable: the connection may already be closed by the time
		// this side notices, which is what "closed rather than left open"
		// means from the other end.
		return
	}
	if resp.Code != string(BadParams) {
		t.Fatalf("code %q, want %q (%s)", resp.Code, BadParams, resp.Error)
	}
}

// A request comfortably inside the budget, and comfortably past what a
// single buffered read would carry, still arrives whole - the fix bounds the
// frame, it does not shrink what a legitimate request may carry.
func TestALegitimateLargeRequestStillSucceeds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.sock")
	srv, err := ListenAt(path, func(method string, params json.RawMessage) (any, error) {
		if method != "big" {
			return nil, errors.New("unhandled")
		}
		return map[string]any{"bytes": len(params)}, nil
	})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	stop := make(chan struct{})
	go func() {
		tick := time.NewTicker(2 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				srv.Pump()
			}
		}
	}()
	t.Cleanup(func() { close(stop); _ = srv.Close() })

	c, err := DialAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	// A megabyte: the size a bulk node list or a long notes field might
	// actually reach, well past a bufio-sized read and well under the limit.
	payload := strings.Repeat("x", 1<<20)
	raw, err := c.Call("big", payload)
	if err != nil {
		t.Fatalf("a legitimate large request was refused: %v", err)
	}
	var got struct {
		Bytes int `json:"bytes"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Bytes < len(payload) {
		t.Fatalf("only %d of the %d bytes sent arrived", got.Bytes, len(payload))
	}
}
