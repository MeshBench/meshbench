package engine_test

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/mesh/firmware"
	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// The wire's own message kinds, mirrored here rather than exported: a test
// double should look like the thing it stands in for, not reach into it.
const (
	fakeKindTick = 0x02
	fakeKindAck  = 0x03
)

// noopBackend is a Backend that never really runs anything - the bridge and
// a raw connection are what this test drives, not a process.
type noopBackend struct{}

func (noopBackend) Start(context.Context, string) error { return nil }
func (noopBackend) Stop() error                         { return nil }
func (noopBackend) Kind() string                        { return "test" }
func (noopBackend) HasConsole() bool                    { return false }
func (noopBackend) ConsoleIn() io.Writer                { return nil }

// fakeFirmwareConn stands in for a firmware process: it acks every tick it is
// sent, until crash ends the connection - the one thing a crashed process and
// a killed one have in common from the bridge's side.
type fakeFirmwareConn struct {
	conn net.Conn
	acks chan uint32
}

func dialFake(t *testing.T, addr string) *fakeFirmwareConn {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial bridge: %v", err)
	}
	f := &fakeFirmwareConn{conn: c, acks: make(chan uint32, 256)}
	go f.serve()
	return f
}

func (f *fakeFirmwareConn) serve() {
	var hdr [3]byte
	for {
		if _, err := readFull(f.conn, hdr[:]); err != nil {
			return
		}
		n := binary.BigEndian.Uint16(hdr[1:])
		buf := make([]byte, n)
		if n > 0 {
			if _, err := readFull(f.conn, buf); err != nil {
				return
			}
		}
		if hdr[0] == fakeKindTick && n >= 4 {
			at := binary.BigEndian.Uint32(buf[:4])
			reply := make([]byte, 3+4)
			reply[0] = fakeKindAck
			binary.BigEndian.PutUint16(reply[1:], 4)
			binary.BigEndian.PutUint32(reply[3:], at)
			if _, err := f.conn.Write(reply); err != nil {
				return
			}
			f.acks <- at
		}
	}
}

func readFull(c net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := c.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func (f *fakeFirmwareConn) crash() { _ = f.conn.Close() }

func waitAttached(t *testing.T, br *firmware.Bridge) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if br.Attached() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("bridge never reported a connection attached")
}

func (f *fakeFirmwareConn) waitAcked(t *testing.T, atLeast int) {
	t.Helper()
	got := 0
	for got < atLeast {
		select {
		case <-f.acks:
			got++
		case <-time.After(5 * time.Second):
			t.Fatalf("only saw %d of %d expected acks", got, atLeast)
		}
	}
}

// One node's firmware dying mid-run must not stop the others ticking.
//
// runFirmware used to return on the first Bridge call that failed. Because
// it walks nodes in a fixed order, that left every node after the dead one
// in that order un-ticked forever, silently, on every Step after - the
// mechanism behind "west lomond is fine, everything after it is not". This
// pins the fix: "dying" is added *first*, so a return-on-first-error
// regression would freeze "alive" too, not just its own node.
func TestOneCrashedFirmwareDoesNotFreezeTheOthers(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10})
	defer func() { _ = e.Close() }()

	dyingBr, err := firmware.Listen("127.0.0.1:0", "dying")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	aliveBr, err := firmware.Listen("127.0.0.1:0", "alive")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	e.Add(node("dying", 56.70, -3.90, 22), &firmware.Node{Bridge: dyingBr, Backend: noopBackend{}})
	e.Add(node("alive", 56.705, -3.905, 22), &firmware.Node{Bridge: aliveBr, Backend: noopBackend{}})

	dying := dialFake(t, dyingBr.Addr())
	alive := dialFake(t, aliveBr.Addr())
	// accept() registers the connection on its own goroutine; Step must not
	// race it, or a bridge that has not caught up yet looks exactly like a
	// crashed one.
	waitAttached(t, dyingBr)
	waitAttached(t, aliveBr)

	ctx := context.Background()
	if err := e.Step(ctx); err != nil {
		t.Fatalf("first step: %v", err)
	}
	dying.waitAcked(t, 1)
	alive.waitAcked(t, 1)

	dying.crash()
	// Real sockets, even on loopback: give the bridge's read loop a moment
	// to notice the close before the next Step leans on it.
	time.Sleep(100 * time.Millisecond)

	for i := 0; i < 10; i++ {
		if err := e.Step(ctx); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	alive.waitAcked(t, 10)

	failures := e.FirmwareFailures()
	if len(failures) != 1 || failures[0] != "dying" {
		t.Fatalf("FirmwareFailures = %v, want exactly [\"dying\"]", failures)
	}
}
