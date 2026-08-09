package companion

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

type fakeNode struct {
	written chan Frame
	out     chan Frame
}

func newFake() *fakeNode {
	return &fakeNode{written: make(chan Frame, 8), out: make(chan Frame, 8)}
}
func (f *fakeNode) Write(fr Frame) error { f.written <- fr; return nil }
func (f *fakeNode) Subscribe() (<-chan Frame, func()) {
	return f.out, func() {}
}

func dial(t *testing.T, s *Server) net.Conn {
	t.Helper()
	c, err := net.Dial("tcp", s.Addr())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func writeFrame(t *testing.T, c net.Conn, b []byte) {
	t.Helper()
	var hdr [2]byte
	binary.BigEndian.PutUint16(hdr[:], uint16(len(b)))
	if _, err := c.Write(append(hdr[:], b...)); err != nil {
		t.Fatal(err)
	}
}

func readFrame(t *testing.T, c net.Conn) []byte {
	t.Helper()
	var hdr [2]byte
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(c, hdr[:]); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, binary.BigEndian.Uint16(hdr[:]))
	if _, err := io.ReadFull(c, buf); err != nil {
		t.Fatal(err)
	}
	return buf
}

func TestFramesRoundTripBothDirections(t *testing.T) {
	n := newFake()
	s, err := Listen("127.0.0.1:0", n, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	c := dial(t, s)
	defer func() { _ = c.Close() }()

	writeFrame(t, c, []byte{0x11, 0x00, 0xA0})
	select {
	case got := <-n.written:
		if string(got) != string([]byte{0x11, 0x00, 0xA0}) {
			t.Errorf("node received % x", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("node never received the client's frame")
	}

	n.out <- Frame{0x22, 0xBB}
	if got := readFrame(t, c); string(got) != string([]byte{0x22, 0xBB}) {
		t.Errorf("client received % x, want 22 bb", got)
	}
}

// Message boundaries must survive a stream transport. Two frames written
// back-to-back must arrive as two frames, not one blob — this is the whole
// reason for the length prefix.
func TestFrameBoundariesArePreserved(t *testing.T) {
	n := newFake()
	s, _ := Listen("127.0.0.1:0", n, nil)
	defer func() { _ = s.Close() }()
	c := dial(t, s)
	defer func() { _ = c.Close() }()

	writeFrame(t, c, []byte{1, 2, 3})
	writeFrame(t, c, []byte{4, 5})

	first := <-n.written
	second := <-n.written
	if len(first) != 3 || len(second) != 2 {
		t.Errorf("boundaries lost: got %d and %d bytes, want 3 and 2", len(first), len(second))
	}
}

// ADR-0008: attaching a real client pins the clock to 1x, and that must be
// surfaced rather than silently applied. The transport reports the transition;
// the simulator decides what to do about it.
func TestAttachTransitionsAreReportedOnce(t *testing.T) {
	n := newFake()
	events := make(chan bool, 8)
	s, _ := Listen("127.0.0.1:0", n, func(attached bool) { events <- attached })
	defer func() { _ = s.Close() }()

	c1 := dial(t, s)
	if got := <-events; got != true {
		t.Error("first attach did not report true")
	}

	// A second client must NOT re-report: the clock is already pinned.
	c2 := dial(t, s)
	select {
	case e := <-events:
		t.Errorf("second connection reported %v; only transitions should fire", e)
	case <-time.After(300 * time.Millisecond):
	}

	_ = c1.Close()
	select {
	case e := <-events:
		t.Errorf("closing one of two clients reported %v; still attached", e)
	case <-time.After(300 * time.Millisecond):
	}

	_ = c2.Close()
	if got := <-events; got != false {
		t.Error("last detach did not report false")
	}
}

// Localhost by default — a simulator that opens forty ports on 0.0.0.0 is a
// surprise nobody asked for.
func TestBindsLoopbackWhenAsked(t *testing.T) {
	n := newFake()
	s, err := Listen("127.0.0.1:0", n, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	host, _, _ := net.SplitHostPort(s.Addr())
	if host != "127.0.0.1" {
		t.Errorf("bound to %s, want loopback", host)
	}
}
