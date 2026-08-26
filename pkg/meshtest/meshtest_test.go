package meshtest_test

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/pkg/meshtest"
)

// The example in the package doc, run.
//
// It needs firmware, which is downloaded on first use, so it is skipped unless
// MESHTEST_LIVE is set - the same bargain the engine's own live tests make. A
// test that quietly downloads a firmware image on somebody's laptop is not a
// unit test.
func TestAMeshCanBeStartedAndTalkedTo(t *testing.T) {
	if os.Getenv("MESHTEST_LIVE") == "" {
		t.Skip("set MESHTEST_LIVE=1: this starts real firmware and may download it")
	}
	m, err := meshtest.Start(context.Background(), meshtest.Options{})
	if err != nil {
		t.Fatalf("starting a mesh: %v", err)
	}
	defer func() { _ = m.Close() }()

	if len(m.Nodes()) == 0 {
		t.Fatal("a mesh with no nodes")
	}
	if m.Elapsed() != 0 {
		t.Fatalf("time had already moved before the test asked: %s", m.Elapsed())
	}

	// Five seconds first: a client that attaches at time zero and waits stalls
	// the run. See the package doc.
	if err := m.Advance(5 * time.Second); err != nil {
		t.Fatalf("settling: %v", err)
	}

	// The endpoint is real: an unmodified client dials it.
	conn, err := net.DialTimeout("tcp", m.Endpoint(), 5*time.Second)
	if err != nil {
		t.Fatalf("dialling the companion at %s: %v", m.Endpoint(), err)
	}
	defer func() { _ = conn.Close() }()

	// Drain it, as a real client does. A companion that connects and never
	// reads fills the link's buffer and the mesh slows to a stop behind it -
	// see TestASilentClientStallsTheMesh.
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := conn.Read(buf); err != nil {
				return
			}
		}
	}()

	if err := m.Advance(55 * time.Second); err != nil {
		t.Fatalf("advancing: %v", err)
	}
	if m.Elapsed() < 60*time.Second {
		t.Fatalf("asked for 60 s, got %s", m.Elapsed())
	}

	// Real firmware adverts unprompted, so a minute of a running mesh has
	// traffic in it. Nothing here asserts a count: what matters is that the
	// air is not silent, which is what a broken harness looks like.
	if len(m.Transmissions("")) == 0 {
		t.Error("no node transmitted in sixty seconds of simulated time")
	}
	if len(m.Received("")) == 0 {
		t.Error("nothing was heard by anyone in sixty seconds")
	}
}

// Options should fail with something a reader can act on, not with a nil
// dereference three frames down.
func TestAMissingFixtureSaysSo(t *testing.T) {
	_, err := meshtest.Start(context.Background(), meshtest.Options{
		Fixture: "a-network-that-does-not-exist",
	})
	if err == nil {
		t.Fatal("a fixture that does not exist started a mesh")
	}
	t.Logf("refused with: %v", err)
}
