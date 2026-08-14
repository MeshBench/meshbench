package firmware_test

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
)

// What a companion does when the text CLI is typed at it.
//
// A sweep of eight cells failed on its first companion with "connection reset
// by peer", and the node was gone afterwards. Provisioning types the same lines
// at every node regardless of role, so this asks the narrow question directly:
// does a companion_radio build survive being spoken to in CLI?
//
//	MESHCORESIM_LIVE=1 go test ./internal/firmware/ -run TestACompanionSurvivesTheCLI -v
func TestACompanionSurvivesTheCLI(t *testing.T) {
	if os.Getenv("MESHCORESIM_LIVE") == "" {
		t.Skip("set MESHCORESIM_LIVE=1")
	}
	if _, err := firmware.FindNative("", "companion_radio"); err != nil {
		t.Skipf("no native companion build: %v", err)
	}

	log := &bytes.Buffer{}
	n, err := firmware.Start(context.Background(), "companion-1",
		&firmware.Native{Seed: 4417, Role: "companion_radio", Log: log})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = n.Close() }()

	deadline := time.Now().Add(10 * time.Second)
	for !n.Bridge.Attached() {
		if time.Now().After(deadline) {
			t.Fatal("the companion never connected to the bridge")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// The lines provisioning sends, in the order it sends them.
	for _, line := range []string{
		"set name Probe",
		"time 1788220800",
		"set flood.max.advert 32",
		"region put sco",
		"region allowf sco",
		"region save",
		"region default sco",
	} {
		if err := n.Bridge.Type([]byte(line + "\r\n")); err != nil {
			t.Fatalf("typing %q at a companion: %v\nnode stderr:\n%s", line, err, log)
		}
		// Time has to move for the firmware to read its serial input.
		if err := n.Bridge.Advance(context.Background(), uint32(200)); err != nil {
			t.Fatalf("advancing after %q: %v\nnode stderr:\n%s", line, err, log)
		}
	}
	if !n.Bridge.Attached() {
		t.Fatalf("the companion left the bridge after the CLI\nnode stderr:\n%s", log)
	}
	t.Logf("the companion took every line; stderr:\n%s", log)
}
