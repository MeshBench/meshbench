package sdr_test

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/sdr"
)

// toneSource streams a constant strong tone, big enough to stand clear of
// the 8-bit midpoint.
type toneSource struct{}

func (toneSource) SampleRateHz() float64 { return 62500 }
func (toneSource) NextSamples(n int) []complex128 {
	out := make([]complex128, n)
	for i := range out {
		out[i] = complex(3e-5, -3e-5)
	}
	return out
}

// The wire format is rtl_tcp's own: the RTL0 header, then unsigned 8-bit IQ,
// with five-byte commands accepted the other way. That is what makes SDR++'s
// stock client connect without MeshBench-specific anything.
func TestRTLTCPSpeaksTheProtocol(t *testing.T) {
	srv, err := sdr.ServeRTLTCP("127.0.0.1:0", toneSource{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()

	conn, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	hdr := make([]byte, 12)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(conn, hdr); err != nil {
		t.Fatal(err)
	}
	if string(hdr[:4]) != "RTL0" {
		t.Fatalf("header magic %q", hdr[:4])
	}

	// Tune and set the rate, as SDR++ does on connect.
	cmd := make([]byte, 5)
	cmd[0] = 0x01
	binary.BigEndian.PutUint32(cmd[1:], 869525000)
	if _, err := conn.Write(cmd); err != nil {
		t.Fatal(err)
	}
	cmd[0] = 0x02
	binary.BigEndian.PutUint32(cmd[1:], 62500)
	if _, err := conn.Write(cmd); err != nil {
		t.Fatal(err)
	}

	// Samples must flow, and the tone must sit off the 127/128 midpoint.
	buf := make([]byte, 4096)
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	off := 0
	for i := 0; i < len(buf); i += 2 {
		if buf[i] > 150 {
			off++
		}
	}
	if off < len(buf)/4 {
		t.Fatalf("the tone did not reach the stream: %d high samples of %d", off, len(buf)/2)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if f, r := srv.Tuned(); f == 869525000 && r == 62500 {
			break
		}
		if time.Now().After(deadline) {
			f, r := srv.Tuned()
			t.Fatalf("commands not recorded: freq=%d rate=%d", f, r)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
