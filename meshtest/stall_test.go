package meshtest_test

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/meshtest"
)

// A client that connects and never reads must not stop the mesh.
//
// This began as a reproduction: sixty seconds of simulated time did not finish
// in two and a half minutes of real time behind a client that had stopped
// taking output. It is a guard now - the same run finishes in about ten
// seconds - and it is kept live-only because it starts real firmware.
func TestASilentClientDoesNotStallTheMesh(t *testing.T) {
	if os.Getenv("MESHTEST_LIVE") == "" {
		t.Skip("set MESHTEST_LIVE=1: this starts real firmware and may download it")
	}
	m, err := meshtest.Start(context.Background(), meshtest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()

	// Connected, and deliberately never read from.
	conn, err := net.DialTimeout("tcp", m.Endpoint(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	done := make(chan error, 1)
	go func() { done <- m.Advance(60 * time.Second) }()
	select {
	case err := <-done:
		t.Logf("after dialling: advanced fine (%v), %d events", err, m.Engine().EventCount())
	case <-time.After(45 * time.Second):
		t.Fatalf("STALLED: 60 s of simulated time did not finish in 45 s of real time "+
			"behind a client that never reads (%d events)", m.Engine().EventCount())
	}
}
