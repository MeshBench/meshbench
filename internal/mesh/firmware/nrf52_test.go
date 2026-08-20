package firmware

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeUF2(t *testing.T, base uint32) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.uf2")
	if err := os.WriteFile(path, uf2blk(base, []byte{1, 2, 3, 4}), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A node that boots into a fill pattern looks like a broken emulator, so the
// missing SoftDevice has to be refused by name instead.
func TestFlashRefusesAMissingSoftDeviceByName(t *testing.T) {
	e := &EmulatedNode{Image: writeUF2(t, 0x26000), SoftDeviceDir: t.TempDir()}
	_, err := e.flashRegions()
	if err == nil {
		t.Fatal("an image linked above an absent SoftDevice was accepted")
	}
	if !strings.Contains(err.Error(), "6.1.1") {
		t.Errorf("the refusal does not name the version: %v", err)
	}
}

func TestFlashRefusesAnUnknownBase(t *testing.T) {
	e := &EmulatedNode{Image: writeUF2(t, 0x31000), SoftDeviceDir: t.TempDir()}
	if _, err := e.flashRegions(); err == nil {
		t.Fatal("an image at an unrecognised base was accepted")
	}
}

// An image based at zero is a whole flash image and needs nothing under it.
func TestFlashTakesAZeroBasedImageAlone(t *testing.T) {
	e := &EmulatedNode{Image: writeUF2(t, 0)}
	plan, err := e.flashRegions()
	if err != nil {
		t.Fatal(err)
	}
	if plan.softDevice != "" {
		t.Errorf("a zero-based image asked for a SoftDevice: %q", plan.softDevice)
	}
}

// One constant device ID would give every emulated nRF52 node in a network the
// same identity, which reads as a mesh problem and is not one.
func TestFICRDeviceIDFollowsTheNodeName(t *testing.T) {
	a := (&EmulatedNode{NodeName: "n1"}).ficrWrites()
	b := (&EmulatedNode{NodeName: "n2"}).ficrWrites()
	if a == b {
		t.Fatal("two nodes were given the same factory device ID")
	}
	if a != (&EmulatedNode{NodeName: "n1"}).ficrWrites() {
		t.Error("the same node name produced two different device IDs")
	}
	if !strings.Contains(a, "0x10000060") {
		t.Error("DEVICEID[0] was not written")
	}
}
