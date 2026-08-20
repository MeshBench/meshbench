package meshtest_test

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/meshtest"
)

// Reproduces the stall described in the package doc: a client that connects
// and never reads.
//
// Live-only and expected to fail until the engine side is fixed - it is a
// reproduction, not a guard. Advancing first is the documented way around it.
func TestASilentClientStallsTheMesh(t *testing.T) {
	if os.Getenv("MESHTEST_STALL") == "" {
		t.Skip("set MESHTEST_STALL=1: reproduces a known stall and takes 45 s to do it")
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
