package firmware_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/firmware/native"
	"github.com/MeshBench/meshbench/internal/mesh/proto"
)

// frame is the companion protocol's own framing: '<', a little-endian length,
// then the payload. The same one the session uses.
func frame(payload []byte) []byte {
	out := make([]byte, 0, 3+len(payload))
	out = append(out, '<')
	out = binary.LittleEndian.AppendUint16(out, uint16(len(payload)))
	return append(out, payload...)
}

// What a companion does when it is spoken to, and what it says on the way out.
//
// Every cell of a sweep died at the sender: first on the repeater CLI, then on
// the AppStart handshake. The experiment discards a node's stderr, so this is
// the only place the firmware's own account of it is visible.
//
//	MESHBENCH_LIVE=1 MESHBENCH_NATIVE=~/msim/meshcore-native/build \
//	go test ./internal/firmware/ -run TestWhatKillsACompanion -v
func TestWhatKillsACompanion(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}
	if _, err := firmware.FindNative("", "companion_radio"); err != nil {
		t.Skipf("no native companion build: %v", err)
	}

	log := &bytes.Buffer{}
	n, err := firmware.Start(context.Background(), "companion-1",
		&native.Native{Seed: 4417, Role: "companion_radio", Log: log})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = n.Close() }()

	deadline := time.Now().Add(10 * time.Second)
	for !n.Bridge.Attached() {
		if time.Now().After(deadline) {
			t.Fatalf("never attached; stderr:\n%s", log)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Log("attached")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// A tick first, with nothing said: does a companion advance at all before
	// its app session exists?
	if err := n.Bridge.SetChannelBusy(false); err != nil {
		t.Fatalf("channel state: %v\nstderr:\n%s", err, log)
	}
	if err := n.Bridge.Advance(ctx, 100); err != nil {
		t.Fatalf("a companion would not advance before AppStart: %v\nstderr:\n%s", err, log)
	}
	t.Log("advanced 100 ms with no session")

	// Then the handshake.
	if err := n.Bridge.Type(frame(proto.AppStart("meshbench"))); err != nil {
		t.Fatalf("AppStart: %v\nstderr:\n%s", err, log)
	}
	for i := 0; i < 5; i++ {
		if err := n.Bridge.SetChannelBusy(false); err != nil {
			t.Fatalf("channel state after AppStart (tick %d): %v\nstderr:\n%s", i, err, log)
		}
		if err := n.Bridge.Advance(ctx, uint32(200*(i+1))); err != nil {
			t.Fatalf("advance after AppStart (tick %d): %v\nstderr:\n%s", i, err, log)
		}
	}
	t.Logf("the companion survived AppStart; stderr:\n%s", log)
}
