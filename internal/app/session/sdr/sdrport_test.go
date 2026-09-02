package sdr

import (
	"net"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
)

// silence stands in for a node's receiver: the listener under test never
// serves a client, so what the stream would carry does not matter.
type silence struct{}

func (silence) NextSamples(n int) []complex128 { return make([]complex128, n) }
func (silence) SampleRateHz() float64          { return 250e3 }
func (silence) NoisePSD() float64              { return 0 }

func freshState() *sdrState {
	return &sdrState{servers: map[string]*sdrServer{}, ports: map[string]int{}}
}

// Serving a node a second time puts it back where it was.
//
// The port was drawn fresh each time, so SDR software pointed at the first
// address went on reading a socket that had been closed, with nothing on
// screen to say the stream had moved.
func TestServingANodeAgainKeepsItsAddress(t *testing.T) {
	ss := freshState()
	first, moved, err := listen(ss, "West Lomond", silence{})
	if err != nil {
		t.Fatalf("the first serve failed: %v", err)
	}
	if moved != "" {
		t.Errorf("the first serve reported a move: %q", moved)
	}
	addr := first.Addr()
	ss.ports["West Lomond"] = session.PortOf(addr)
	if err := first.Close(); err != nil {
		t.Fatalf("closing the first listener: %v", err)
	}

	second, moved, err := listen(ss, "West Lomond", silence{})
	if err != nil {
		t.Fatalf("the second serve failed: %v", err)
	}
	defer func() { _ = second.Close() }()
	if moved != "" {
		t.Errorf("the second serve reported a move: %q", moved)
	}
	if second.Addr() != addr {
		t.Errorf("served again at %s, having been at %s", second.Addr(), addr)
	}
}

// A port taken since is not a reason to fail, and not a reason to say nothing
// either: the endpoint moves, and the move is reported.
func TestATakenPortMovesTheEndpointAndSaysSo(t *testing.T) {
	ss := freshState()
	squatter, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not take a port to squat on: %v", err)
	}
	defer func() { _ = squatter.Close() }()
	ss.ports["West Lomond"] = session.PortOf(squatter.Addr().String())

	srv, moved, err := listen(ss, "West Lomond", silence{})
	if err != nil {
		t.Fatalf("a taken port stopped the serve outright: %v", err)
	}
	defer func() { _ = srv.Close() }()
	if moved == "" {
		t.Error("the endpoint moved and nothing said so")
	}
	if srv.Addr() == squatter.Addr().String() {
		t.Error("the serve landed on the port something else holds")
	}
}
