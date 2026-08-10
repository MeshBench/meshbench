package engine_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/A13xB0/meshcoresim/internal/antenna"
	"github.com/A13xB0/meshcoresim/internal/engine"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// What does a real companion_radio build actually put on its serial port?
// The end-to-end companion check, and the two bugs it holds down: a transport
// that re-framed MeshCore's already-framed serial stream, and a workbench
// console that took the port back from an attached client on the next frame.
// Both presented identically — a client that connects and then receives
// nothing — which is why this asserts on bytes reaching a real socket.
func TestLiveCompanionSerialBytes(t *testing.T) {
	if os.Getenv("MESHCORESIM_LIVE") == "" {
		t.Skip("set MESHCORESIM_LIVE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	e := engine.New(flat{100}, engine.Config{
		FreqMHz: 869.618, SF: 8, BandwidthHz: 62500, CodingRate: 4,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417,
	})
	defer func() { _ = e.Close() }()

	e.Add(scenario.Node{
		Name: "phone", Kind: scenario.Companion,
		Position: scenario.LatLon{Lat: 56.7, Lon: -3.9}, HeightAGLm: 2,
		Antenna:    antenna.Mounted{Pattern: antenna.Dipole{}, Polarisation: "vertical"},
		TxPowerDBm: 14, NoiseFigureDB: 6,
		Radio: scenario.RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62500, SpreadFactor: 8, CodingRate: 4},
	}, nil)

	if err := e.AttachNative(ctx, 4417); err != nil {
		t.Fatal(err)
	}
	n, _ := e.NodeByName("phone")
	var out serialBuf
	n.Firmware.Bridge.Console(&out)
	if err := e.Run(ctx, 3000); err != nil {
		t.Fatal(err)
	}

	b := out.bytes()
	t.Logf("%d bytes on serial", len(b))
	if len(b) > 0 {
		show := b
		if len(show) > 64 {
			show = show[:64]
		}
		t.Logf("first bytes: % X", show)
		t.Logf("as text: %q", string(show))
	}

	// The end-to-end check: a client on the TCP port, speaking MeshCore's own
	// framing, must get the firmware's own framed reply back — with nothing
	// added, removed, or stolen by the workbench console.
	link, err := e.ServeCompanionTCP("phone", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = link.Close() }()

	conn, err := net.Dial("tcp", link.Addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	// The console tries to take the port back on every frame. It must not be
	// able to: that is precisely what made a connected client see nothing.
	n.Firmware.Bridge.Console(&out)
	if !n.Firmware.Bridge.Claimed() {
		t.Fatal("the companion link did not claim the serial port")
	}

	if _, err := conn.Write([]byte{'<', 0x01, 0x00, 0x01}); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(ctx, 6000); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	reply := make([]byte, 5)
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatalf("no reply reached the client: %v", err)
	}
	// '>' then a little-endian length: MeshCore's own framing, intact.
	if reply[0] != '>' {
		t.Errorf("reply does not start with MeshCore's frame marker: % X", reply)
	}
	if int(reply[1])|int(reply[2])<<8 != len(reply)-3 {
		t.Errorf("frame length does not match the payload: % X", reply)
	}
	fmt.Printf("client received % X\n", reply)
}

// serialBuf collects serial output from the bridge's goroutine.
type serialBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *serialBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *serialBuf) bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.b.Bytes()...)
}
