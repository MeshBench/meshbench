package session

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

func repeaterNode(name string) scenario.Node {
	return scenario.Node{
		Name: name, Kind: scenario.SimpleRepeater,
		Position: scenario.LatLon{Lat: 56.3, Lon: -3.2},
	}
}

// The default provisioning sends what the old workbench sends at attach.
func TestDefaultProvisioningMatchesOldWorkbench(t *testing.T) {
	cmds := DefaultProvisioning().CommandsFor(repeaterNode("Abernethy Repeater"))
	joined := strings.Join(cmds, "\n")
	for _, want := range []string{
		"set name Abernethy Repeater",
		"set lat 56.300000",
		"set lon -3.200000",
		"time 1788220800",
		"set flood.max.advert 32",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("default provisioning is missing %q:\n%s", want, joined)
		}
	}
	// The pass-through knobs stay silent until somebody sets them: -1 and
	// empty mean the firmware's own defaults, not zero.
	for _, absent := range []string{"path.hash.mode", "loop.detect", "set cad"} {
		if strings.Contains(joined, absent) {
			t.Errorf("default provisioning sends %q without being asked:\n%s", absent, joined)
		}
	}
}

// The knobs, once set, produce the firmware's own commands.
func TestStartupKnobsPassThrough(t *testing.T) {
	p := DefaultProvisioning()
	p.PathHashMode = 1
	p.LoopDetect = "minimal"
	p.CadMode = "aggressive"
	joined := strings.Join(p.CommandsFor(repeaterNode("A")), "\n")
	for _, want := range []string{
		"set path.hash.mode 1",
		"set loop.detect minimal",
		"set cad aggressive",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q:\n%s", want, joined)
		}
	}
}

// A name longer than the firmware's field is cut on a rune boundary, never
// inside an emoji.
func TestNameTruncatesOnRunes(t *testing.T) {
	long := "West Lomond " + strings.Repeat("⛰", 30)
	cmds := DefaultProvisioning().CommandsFor(repeaterNode(long))
	var sent string
	for _, c := range cmds {
		if strings.HasPrefix(c, "set name ") {
			sent = strings.TrimPrefix(c, "set name ")
		}
	}
	if got := len([]rune(sent)); got != 32 {
		t.Errorf("sent a %d-rune name, want 32", got)
	}
	if !strings.HasPrefix(sent, "West Lomond ") {
		t.Errorf("the name lost its head: %q", sent)
	}
	for _, r := range sent {
		if r == '\uFFFD' {
			t.Fatalf("the cut landed inside a rune: %q", sent)
		}
	}
}

// StaggerBoot stays the default. Started together, several hundred nodes
// transmit together, and the collision storm that follows is a burst no real
// network ever sees - pinned so a refactor cannot quietly turn it off.
func TestStaggerBootIsTheDefault(t *testing.T) {
	e := engine.New(bareEarth{}, engine.Config{
		FreqMHz: 869.618, SF: 10, BandwidthHz: 250e3, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: 1,
	})
	defer func() { _ = e.Close() }()
	if !e.StaggerBoot {
		t.Fatal("engine.New no longer staggers boots by default")
	}
}
