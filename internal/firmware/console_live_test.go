package firmware_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
)

// safeBuf collects console output written from the bridge's goroutine.
type safeBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// The console is a real UART or it is nothing. This starts a published MeshCore
// build, reads what it prints on startup, types a command at it, and requires
// the firmware's own reply — no part of which this side composes.
func TestLiveConsoleReachesTheFirmware(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cat := &firmware.NativeCatalogue{CacheDir: t.TempDir()}
	path, err := cat.Ensure(ctx, "simple_repeater", "main")
	if err != nil {
		t.Fatal(err)
	}

	node, err := firmware.Start(ctx, "console-test", &firmware.Native{
		Path: path, Seed: 4417, SF: 10, BandwidthKHz: 250, CodingRate: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = node.Close() }()

	var out safeBuf
	node.Bridge.Console(&out)

	for deadline := time.Now().Add(10 * time.Second); !node.Bridge.Attached(); {
		if time.Now().After(deadline) {
			t.Fatal("the node never connected back to the bridge")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Time has to move for the application to reach its loop, because the
	// simulator owns the clock. Without ticking, nothing ever runs.
	tick := func(ms uint32) {
		if err := node.Bridge.Advance(ctx, ms); err != nil {
			t.Fatalf("tick to %d ms: %v", ms, err)
		}
	}
	tick(100)

	// A repeater prints its public key at startup. Nothing on this side knows
	// what that key is, which is what makes it evidence the firmware ran.
	if got := out.String(); !strings.Contains(got, "Repeater ID:") {
		t.Errorf("startup output did not come from the repeater application:\n%s", got)
	}

	if err := node.Bridge.Type([]byte("ver\r\n")); err != nil {
		t.Fatal(err)
	}
	for at := uint32(200); at <= 2000 && !strings.Contains(out.String(), "->"); at += 100 {
		tick(at)
	}

	got := out.String()
	// MeshCore's CLI echoes what it was given and prefixes its reply with "->".
	// Both halves matter: the echo proves every character survived the trip, and
	// an earlier bug in the serial shim destroyed alternate ones so that `ver`
	// arrived as `e` and drew a perfectly plausible "Unknown command".
	if !strings.Contains(got, "ver\n") && !strings.Contains(got, "ver\r") {
		t.Errorf("the command did not reach the firmware intact:\n%s", got)
	}
	if !strings.Contains(got, "->") || strings.Contains(got, "Unknown command") {
		t.Errorf("the CLI did not answer `ver`:\n%s", got)
	}
	t.Logf("console said:\n%s", got)
}
