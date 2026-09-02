package companion

import (
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// fakeSerial is a node's port that records what was attached.
type fakeSerial struct {
	mu sync.Mutex
	w  io.Writer
}

func (f *fakeSerial) Write([]byte) error { return nil }
func (f *fakeSerial) Attach(w io.Writer) { f.mu.Lock(); f.w = w; f.mu.Unlock() }
func (f *fakeSerial) Detach()            { f.mu.Lock(); f.w = nil; f.mu.Unlock() }

// A client that connects and stops reading must not stop the firmware.
//
// Before the write was bounded, this blocked for as long as the client stayed
// attached: the socket's send buffer filled, Write blocked inside it, and the
// engine behind it waited. Sixty seconds of simulated time did not finish in
// two and a half minutes of real time.
func TestAClientThatStopsReadingDoesNotBlockTheFirmware(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			accepted <- c
		}
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately never read from: that is the whole case.
	defer func() { _ = client.Close() }()

	server := <-accepted
	defer func() { _ = server.Close() }()

	// Small buffers at both ends, so "more than any socket buffer holds"
	// below is a fact rather than a hope. Windows autotunes loopback
	// buffers and is generous with them: it swallowed all four megabytes
	// without once blocking, so nothing stalled and the test failed for the
	// writes having succeeded - which is the opposite of what it looks like.
	if c, ok := client.(*net.TCPConn); ok {
		_ = c.SetReadBuffer(4 << 10)
	}
	if c, ok := server.(*net.TCPConn); ok {
		_ = c.SetWriteBuffer(4 << 10)
	}

	p := NewPipe(&fakeSerial{})
	var stalls int
	var mu sync.Mutex
	p.OnStall = func() { mu.Lock(); stalls++; mu.Unlock() }
	p.attach(server)

	// Far more than any socket buffer holds, so a blocking write cannot get
	// through it however generous the kernel is feeling.
	chunk := make([]byte, 64<<10)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 64; i++ {
			_, _ = p.Write(chunk)
		}
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("writing to a client that stopped reading blocked the firmware")
	}

	if !p.Stalled() {
		t.Error("the pipe does not report the client as stalled")
	}
	mu.Lock()
	got := stalls
	mu.Unlock()
	// Once per episode, not once per write. Not exactly one: a socket window
	// can reopen briefly between stalls and each reopening is a new episode,
	// which is honest. What matters is that sixty-four blocked writes do not
	// produce sixty-four reports.
	if got == 0 {
		t.Error("a client that stopped reading was never reported")
	}
	if got > 8 {
		t.Errorf("OnStall fired %d times across 64 writes; it should report an "+
			"episode, not a write", got)
	}
}

// A client that is reading must still receive everything, unchanged.
func TestAReadingClientIsUnaffected(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	accepted := make(chan net.Conn, 1)
	go func() {
		if c, err := ln.Accept(); err == nil {
			accepted <- c
		}
	}()
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	server := <-accepted
	defer func() { _ = server.Close() }()

	read := make(chan int, 1)
	go func() {
		n, _ := io.Copy(io.Discard, client)
		read <- int(n)
	}()

	p := NewPipe(&fakeSerial{})
	p.OnStall = func() { t.Error("a reading client was reported as stalled") }
	p.attach(server)

	const n = 200
	payload := []byte("the quick brown fox\r\n")
	for i := 0; i < n; i++ {
		if _, err := p.Write(payload); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	if p.Stalled() {
		t.Error("a reading client is reported as stalled")
	}
	_ = server.Close()
	select {
	case got := <-read:
		if want := n * len(payload); got != want {
			t.Errorf("client received %d bytes, wrote %d", got, want)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the client never finished reading")
	}
	_ = errors.New("")
}
