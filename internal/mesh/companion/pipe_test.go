package companion_test

import (
	"bytes"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/mesh/companion"
)

// fakeSerial stands in for a node's UART.
type fakeSerial struct {
	mu      sync.Mutex
	written []byte
	out     io.Writer
}

func (f *fakeSerial) Write(b []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.written = append(f.written, b...)
	return nil
}

func (f *fakeSerial) Attach(w io.Writer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.out = w
}

func (f *fakeSerial) Detach() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.out = nil
}

func (f *fakeSerial) emit(b []byte) {
	f.mu.Lock()
	w := f.out
	f.mu.Unlock()
	if w != nil {
		_, _ = w.Write(b)
	}
}

func (f *fakeSerial) got() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]byte(nil), f.written...)
}

// The bug this exists to prevent: a transport that adds its own length prefix
// on top of MeshCore's own '>' / '<' framing produces a stream no real client
// can parse, and the failure is silent — the client simply never sees a valid
// frame.
//
// So the contract is byte-for-byte transparency in both directions.
func TestTCPLinkIsByteTransparent(t *testing.T) {
	fs := &fakeSerial{}
	l, err := companion.ListenTCP("127.0.0.1:0", fs)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()

	c, err := net.Dial("tcp", l.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	// A client-to-firmware frame, exactly as MeshCore writes it: '<', then a
	// little-endian length, then the payload.
	frame := []byte{'<', 0x03, 0x00, 0xAA, 0xBB, 0xCC}
	if _, err := c.Write(frame); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !bytes.Equal(fs.got(), frame) {
		if time.Now().After(deadline) {
			t.Fatalf("the firmware received %v, the client sent %v", fs.got(), frame)
		}
		time.Sleep(5 * time.Millisecond)
	}

	// And the other way: what the firmware prints must arrive unaltered.
	reply := []byte{'>', 0x02, 0x00, 0x10, 0x20}
	fs.emit(reply)
	buf := make([]byte, len(reply))
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(c, buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf, reply) {
		t.Errorf("the client received %v, the firmware sent %v", buf, reply)
	}
}

// A virtual serial device is the transport most client software can actually
// use, and it must be just as transparent.
func TestSerialLinkIsByteTransparent(t *testing.T) {
	fs := &fakeSerial{}
	l, err := companion.OpenSerial(fs)
	if err != nil {
		t.Skipf("no pty available here: %v", err)
	}
	defer func() { _ = l.Close() }()

	dev, err := openDevice(l.Path())
	if err != nil {
		t.Skipf("cannot open %s: %v", l.Path(), err)
	}
	defer func() { _ = dev.Close() }()

	frame := []byte{'<', 0x01, 0x00, 0x7F}
	if _, err := dev.Write(frame); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !bytes.Equal(fs.got(), frame) {
		if time.Now().After(deadline) {
			t.Fatalf("the firmware received %v, the client sent %v", fs.got(), frame)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// The ADR-0008 regression this guards: the speed pin keys off a client being
// attached, so Attached must flip when one connects to an already-open
// listener and flip back when it leaves.
func TestAttachedTracksTheClient(t *testing.T) {
	fs := &fakeSerial{}
	l, err := companion.ListenTCP("127.0.0.1:0", fs)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()

	if l.Attached() {
		t.Fatal("attached before any client connected")
	}
	c, err := net.Dial("tcp", l.Addr())
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !l.Attached() {
		if time.Now().After(deadline) {
			t.Fatal("client connected but Attached stayed false")
		}
		time.Sleep(5 * time.Millisecond)
	}
	_ = c.Close()
	deadline = time.Now().Add(2 * time.Second)
	for l.Attached() {
		if time.Now().After(deadline) {
			t.Fatal("client left but Attached stayed true")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
