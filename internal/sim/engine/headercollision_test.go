package engine_test

import (
	"context"
	"encoding/binary"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// Where a collision lands decides what the chip reports. A hit on the header
// block is a header the receiver could not read - HeaderErr, and no RxDone -
// while a hit after it is a payload that failed its CRC. Firmware takes a
// different path for each, so the ledger has to tell them apart.
//
// At SF8 and 125 kHz a symbol is 2.048 ms, the 32-symbol preamble with its
// sync and start of frame ends 74 ms in, and the header block runs to 91 ms.
func TestACollisionOnTheHeaderIsAHeaderError(t *testing.T) {
	detail := func(interfererStartMs uint32) string {
		t.Helper()
		e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 4417})
		e.Add(wfNode("a", 0, 22), nil)
		e.Add(wfNode("b", 0.010, 22), nil)
		e.Add(wfNode("c", 0.020, 22), nil)

		frame := make([]byte, 40)
		for i := range frame {
			frame[i] = byte(37 + i*11)
		}
		if err := e.Run(context.Background(), 10); err != nil {
			t.Fatal(err)
		}
		e.InjectFrame(0, frame)
		if err := e.Run(context.Background(), interfererStartMs); err != nil {
			t.Fatal(err)
		}
		e.InjectFrame(2, frame[:20])
		if err := e.Run(context.Background(), 8000); err != nil {
			t.Fatal(err)
		}
		for _, ev := range e.Events() {
			if ev.From == "a" && ev.To == "b" {
				if ev.Kind != "miss" {
					t.Fatalf("an equal-power collision it could not capture over decoded anyway (%s: %s)",
						ev.Kind, ev.Detail)
				}
				return ev.Detail
			}
		}
		t.Fatal("b recorded nothing for a's packet")
		return ""
	}

	if d := detail(60); !strings.Contains(d, "header symbols were destroyed") {
		t.Fatalf("a collision across the header block was not reported as a header error: %q", d)
	}
	if d := detail(200); !strings.Contains(d, "decoded its header") {
		t.Fatalf("a collision after the header block was reported as something else: %q", d)
	}
}

// And the firmware is told. A receiver running firmware is handed the failure
// as the chip would raise it - header or CRC, the first byte of the message -
// rather than nothing, which is all it was ever handed before.
func TestTheFirmwareIsToldWhereAFrameFailed(t *testing.T) {
	told := func(interfererStartMs uint32) []byte {
		t.Helper()
		e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 4417})
		defer func() { _ = e.Close() }()
		br, err := firmware.Listen("127.0.0.1:0", "b")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		e.Add(wfNode("a", 0, 22), nil)
		e.Add(wfNode("b", 0.010, 22), &firmware.Node{Bridge: br, Backend: noopBackend{}})
		e.Add(wfNode("c", 0.020, 22), nil)
		peer := dialRecorder(t, br.Addr())
		defer func() { _ = peer.conn.Close() }()
		for i := 0; i < 200 && !br.Attached(); i++ {
			time.Sleep(5 * time.Millisecond)
		}

		frame := make([]byte, 40)
		for i := range frame {
			frame[i] = byte(37 + i*11)
		}
		if err := e.Run(context.Background(), 10); err != nil {
			t.Fatal(err)
		}
		e.InjectFrame(0, frame)
		if err := e.Run(context.Background(), interfererStartMs); err != nil {
			t.Fatal(err)
		}
		e.InjectFrame(2, frame[:20])
		if err := e.Run(context.Background(), 8000); err != nil {
			t.Fatal(err)
		}
		return peer.failed()
	}

	if got := told(60); len(got) != 1+40 || got[0] != byte(firmware.FailedHeader) {
		t.Fatalf("a header destroyed by a collision reached the firmware as % x, want a header failure", got)
	}
	if got := told(200); len(got) != 1+40 || got[0] != byte(firmware.FailedCRC) {
		t.Fatalf("a payload destroyed by a collision reached the firmware as % x, want a CRC failure", got)
	}
}

// A frame swamped by an interferer it could not capture over is told to the
// chip by when the interferer arrived. Arriving while the preamble still had to
// be found, it left the receiver nothing to lock onto; arriving after the
// header, it found a receiver that had locked and read the header, and the
// payload failed its CRC.
func TestAFrameSwampedBeforeLockIsOneTheChipNeverLockedOnto(t *testing.T) {
	told := func(interfererStartMs uint32) []byte {
		t.Helper()
		e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 4417})
		defer func() { _ = e.Close() }()
		br, err := firmware.Listen("127.0.0.1:0", "b")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		e.Add(wfNode("a", 0, 22), nil)
		e.Add(wfNode("b", 0.010, 22), &firmware.Node{Bridge: br, Backend: noopBackend{}})
		e.Add(wfNode("c", 0.011, 30), nil)
		peer := dialRecorderSaying(t, br.Addr(), 0x03)
		defer func() { _ = peer.conn.Close() }()
		for i := 0; i < 200 && !br.Attached(); i++ {
			time.Sleep(5 * time.Millisecond)
		}
		if err := e.Run(context.Background(), 10); err != nil {
			t.Fatal(err)
		}
		e.InjectFrame(0, make([]byte, 40))
		if err := e.Run(context.Background(), interfererStartMs); err != nil {
			t.Fatal(err)
		}
		e.InjectFrame(2, make([]byte, 60))
		if err := e.Run(context.Background(), 8000); err != nil {
			t.Fatal(err)
		}
		return peer.failed()
	}

	if got := told(30); len(got) == 0 || got[0] != byte(firmware.FailedNoLock) {
		t.Fatalf("a frame swamped during its preamble reached the chip as % x, want never locked onto", got)
	}
	if got := told(120); len(got) == 0 || got[0] != byte(firmware.FailedCRC) {
		t.Fatalf("a frame swamped after its header reached the chip as % x, want a CRC failure", got)
	}
}

// recorder acks every tick, as firmware does, says it takes failed frames,
// and keeps the first one it is handed.
type recorder struct {
	conn net.Conn
	caps byte
	mu   sync.Mutex
	got  []byte
}

func dialRecorder(t *testing.T, addr string) *recorder {
	t.Helper()
	return dialRecorderSaying(t, addr, 0x01)
}

// dialRecorderSaying is dialRecorder with the capability byte chosen.
func dialRecorderSaying(t *testing.T, addr string, caps byte) *recorder {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial bridge: %v", err)
	}
	r := &recorder{conn: c, caps: caps}
	go r.serve()
	return r
}

func (r *recorder) serve() {
	var hdr [3]byte
	for {
		if _, err := readFull(r.conn, hdr[:]); err != nil {
			return
		}
		buf := make([]byte, binary.BigEndian.Uint16(hdr[1:]))
		if len(buf) > 0 {
			if _, err := readFull(r.conn, buf); err != nil {
				return
			}
		}
		switch {
		case hdr[0] == fakeKindTick && len(buf) >= 4:
			// A radio stats record that says it takes failed frames, ahead of
			// the ack, as a native bridge sends one with every tick.
			record := make([]byte, 48)
			record[47] = r.caps
			msg := append([]byte{0x09, 0, 48}, record...)
			msg = append(msg, fakeKindAck, 0, 4, buf[0], buf[1], buf[2], buf[3])
			if _, err := r.conn.Write(msg); err != nil {
				return
			}
		case hdr[0] == 0x0A:
			r.mu.Lock()
			if r.got == nil {
				r.got = buf
			}
			r.mu.Unlock()
		}
	}
}

func (r *recorder) failed() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.got
}
