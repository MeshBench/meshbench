package native_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/firmware/fakenative"
	"github.com/MeshBench/meshbench/internal/firmware/native"
)

// TestMain lets this binary be re-entered as the node it is testing.
//
// The environment is read before anything else, and in particular before the
// testing package parses flags: the backend launches a node with MeshCore's
// arguments, and a test binary handed --bridge would refuse to start at all.
func TestMain(m *testing.M) {
	if fakenative.Mode() != "" {
		os.Exit(fakenative.Serve())
	}
	os.Exit(m.Run())
}

// startFake brings up one stand-in node and its bridge, with the bridge
// separate from the node so a test can close either one first. Which order a
// shutdown happens in is most of what these tests are about.
func startFake(t *testing.T, mode string) (*native.Native, *firmware.Bridge, *syncBuf) {
	t.Helper()
	t.Setenv(fakenative.EnvMode, mode)
	br, err := firmware.Listen("127.0.0.1:0", "fake-node")
	if err != nil {
		t.Fatal(err)
	}
	log := &syncBuf{}
	n := &native.Native{Path: fakenative.Path(), Log: log}
	if err := n.Start(context.Background(), br.Addr()); err != nil {
		_ = br.Close()
		t.Fatalf("start the stand-in node: %v", err)
	}
	t.Cleanup(func() {
		_ = br.Close()
		_ = n.Stop()
		if t.Failed() {
			t.Logf("node stderr:\n%s", log)
		}
	})
	return n, br, log
}

// syncBuf collects a node's stderr.
//
// Locked, because os/exec copies a child's output on a goroutine of its own
// and a test that reads a plain buffer while that goroutine writes it is a
// data race - which the detector reports against the test rather than against
// anything it was meant to be checking.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func (s *syncBuf) saidAll(t *testing.T, want ...string) {
	t.Helper()
	got := s.String()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("node never said %q; stderr:\n%s", w, got)
		}
	}
}

// waitSaid blocks until the node has printed want, which is the only evidence
// available that its process reached a particular point.
func waitSaid(t *testing.T, log *syncBuf, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(log.String(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("the node never said %q; stderr:\n%s", want, log)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// blockingWriter never returns from a write.
//
// A node's log is whatever the caller handed the backend, and the engine hands
// it a file on whatever filesystem the operator keeps their cache on. os/exec
// copies the child's stderr onto it from a goroutine, and cmd.Wait waits for
// that goroutine - so a write that does not return holds the whole shutdown
// open long after the process itself is gone.
type blockingWriter struct{ release chan struct{} }

func (w blockingWriter) Write(p []byte) (int, error) {
	<-w.release
	return len(p), nil
}
