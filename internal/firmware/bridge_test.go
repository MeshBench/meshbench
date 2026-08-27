package firmware

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/rf/channel"
	"github.com/MeshBench/meshbench/internal/rf/dsp"
)

// Stands in for the SX1262 peripheral: the emulator side of the wire.
func fakeRadio(t *testing.T, addr string) net.Conn {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func sendFrame(t *testing.T, c net.Conn, payload []byte) {
	t.Helper()
	hdr := []byte{kindFrame, byte(len(payload) >> 8), byte(len(payload))}
	if _, err := c.Write(append(hdr, payload...)); err != nil {
		t.Fatal(err)
	}
}

func TestEmulatedTransmitReachesTheEngine(t *testing.T) {
	b, err := Listen("127.0.0.1:0", "GB7XYZ")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()

	c := fakeRadio(t, b.Addr())
	defer func() { _ = c.Close() }()
	sendFrame(t, c, []byte{0x11, 0x00, 0xA0, 0xA1})

	select {
	case got := <-b.Transmitted:
		if string(got) != string([]byte{0x11, 0x00, 0xA0, 0xA1}) {
			t.Errorf("engine received % x", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("frame from the emulated radio never reached the RF engine")
	}
}

func TestDeliveryReachesTheEmulator(t *testing.T) {
	b, _ := Listen("127.0.0.1:0", "GB7XYZ")
	defer func() { _ = b.Close() }()
	c := fakeRadio(t, b.Addr())
	defer func() { _ = c.Close() }()

	// Wait for accept.
	for i := 0; i < 50 && !b.Attached(); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if err := b.Deliver([]byte{0x22, 0xBB}); err != nil {
		t.Fatal(err)
	}

	var hdr [3]byte
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(c, hdr[:]); err != nil {
		t.Fatal(err)
	}
	if hdr[0] != kindFrame {
		t.Errorf("wrong frame kind 0x%02x", hdr[0])
	}
	buf := make([]byte, binary.BigEndian.Uint16(hdr[1:]))
	if _, err := io.ReadFull(c, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != string([]byte{0x22, 0xBB}) {
		t.Errorf("emulator received % x, want 22 bb", buf)
	}
}

// The point of the bridge: an emulated node's frame goes through the SAME
// channel as a native one. If it took a different path the two backends could
// not be compared, which is the whole reason ADR-0010 keeps both.
func TestEmulatedFrameGoesThroughTheRealChannel(t *testing.T) {
	b, _ := Listen("127.0.0.1:0", "GB7XYZ")
	defer func() { _ = b.Close() }()
	c := fakeRadio(t, b.Addr())
	defer func() { _ = c.Close() }()

	payload := []byte{0x11, 0x22, 0x33, 0x44}
	sendFrame(t, c, payload)

	var frame []byte
	select {
	case frame = <-b.Transmitted:
	case <-time.After(2 * time.Second):
		t.Fatal("no frame")
	}

	// Modulate it and put it through the channel, exactly as a native node's
	// frame would be.
	const sf = 8
	symbols := make([]int, 0, len(frame))
	for _, by := range frame {
		symbols = append(symbols, int(by))
	}
	wave := dsp.Modulator{SF: sf}.Modulate(symbols)
	obs := channel.Observe([]channel.Transmission{
		{Node: b.Node(), Samples: wave, GainDB: -3, DelaySamples: 0.4},
	}, channel.Receiver{NoisePowerLinear: 0.001, Seed: 4417}, len(wave))

	got := dsp.Demodulator{SF: sf}.Demodulate(obs)
	if len(got) != len(symbols) {
		t.Fatalf("recovered %d symbols, sent %d", len(got), len(symbols))
	}
	for i := range symbols {
		if got[i] != symbols[i] {
			t.Errorf("symbol %d: emulated frame corrupted through the channel: got %d want %d",
				i, got[i], symbols[i])
		}
	}
}

// A second emulator on one bridge is a wiring mistake, not something to
// silently interleave.
func TestSecondEmulatorIsRefused(t *testing.T) {
	b, _ := Listen("127.0.0.1:0", "GB7XYZ")
	defer func() { _ = b.Close() }()
	c1 := fakeRadio(t, b.Addr())
	defer func() { _ = c1.Close() }()
	for i := 0; i < 50 && !b.Attached(); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	c2, err := net.Dial("tcp", b.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c2.Close() }()
	// The refused connection is closed by the server; a write then fails.
	time.Sleep(100 * time.Millisecond)
	_ = c2.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
	_, _ = c2.Write([]byte{kindFrame, 0, 1, 0xFF})
	select {
	case f := <-b.Transmitted:
		t.Errorf("second emulator's frame was accepted: % x", f)
	case <-time.After(300 * time.Millisecond):
	}
}

// A stale release must not clear whoever claimed since.
//
// The workbench's disconnect and its serve race on separate goroutines, and
// when serve won, the disconnect's release unplugged the client that had just
// taken the port - a client that connected and then received nothing.
func TestAStaleReleaseKeepsTheNewClaim(t *testing.T) {
	b, err := Listen("127.0.0.1:0", "GB7XYZ")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()

	first := &struct{ io.Writer }{io.Discard}
	second := &struct{ io.Writer }{io.Discard}
	release1 := b.Claim(first)
	release2 := b.Claim(second)
	release1()
	if !b.Claimed() {
		t.Fatal("releasing a stale claim unplugged the current holder")
	}
	release2()
	if b.Claimed() {
		t.Fatal("the current holder's own release did not release")
	}
	// And a release must stay inert once its claim is gone, however many
	// times it is called.
	release3 := b.Claim(first)
	release1()
	release2()
	if !b.Claimed() {
		t.Fatal("an old release cleared an unrelated later claim")
	}
	release3()
	if b.Claimed() {
		t.Fatal("the final release did not release")
	}
}

// A backend with its own serial port writes through ConsoleSink, and what it
// writes has to land wherever the port currently belongs.
//
// The three states matter separately: nobody listening, the console pane
// attached, and a client holding an exclusive claim. Getting the last one
// wrong is how an attached companion client would receive the boot chain it
// cannot parse, or nothing at all.
func TestConsoleSinkFollowsWhoeverHoldsThePort(t *testing.T) {
	b, err := Listen("127.0.0.1:0", "GB7XYZ")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()

	sink := b.ConsoleSink()
	// Nobody listening: accepted and discarded, never an error - the node has
	// to keep running when no window is open.
	if n, err := sink.Write([]byte("before anyone looked\n")); err != nil || n == 0 {
		t.Fatalf("write with no listener: n=%d err=%v", n, err)
	}

	var pane, client bytes.Buffer
	b.Console(&pane)
	if _, err := sink.Write([]byte("boot ok\n")); err != nil {
		t.Fatal(err)
	}
	release := b.Claim(&client)
	if _, err := sink.Write([]byte("claimed\n")); err != nil {
		t.Fatal(err)
	}
	// The pane re-attaches itself every frame, and must not take the port back.
	b.Console(&pane)
	if _, err := sink.Write([]byte("still claimed\n")); err != nil {
		t.Fatal(err)
	}
	release()
	b.Console(&pane)
	if _, err := sink.Write([]byte("released\n")); err != nil {
		t.Fatal(err)
	}

	if got, want := pane.String(), "boot ok\nreleased\n"; got != want {
		t.Errorf("the pane saw %q, want %q", got, want)
	}
	if got, want := client.String(), "claimed\nstill claimed\n"; got != want {
		t.Errorf("the client saw %q, want %q", got, want)
	}
}
