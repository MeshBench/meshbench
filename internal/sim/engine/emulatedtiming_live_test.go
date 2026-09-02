package engine_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware/emulated"
	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// The measurement behind the claim that an emulated node has no reproducible
// timing, and the harness for anybody who sets out to give it one.
//
// One node, one seed, three runs, and the simulated instant its first
// transmission landed on. A native node put through the same shape answers with
// the same instant every time - that is what the engine's lockstep buys, and
// TestRadioStackIsDeterministic already holds it. An emulated node is ticked
// through the same code path and does not, because the tick is acknowledged by
// the chip model on this side of the socket while the firmware it is meant to
// be stepping runs on the emulator's own clock.
//
// Three runs, not two: two that differ prove nothing more than one bad draw,
// and two that agree would be read as a guarantee. What this asserts is only
// what must hold whatever the three instants turn out to be - that the
// scenario reports itself as not reproducible, so nothing downstream quotes its
// timings against another run's. The instants themselves are logged, because
// the number that matters here is the spread and a passing test that printed
// nothing would leave it unmeasured.
//
// What it read the last time it was run, on a twelve-core machine: 49.83 s,
// 45.72 s and 55.86 s. Recorded because the assertion cannot carry it, and
// because a later run that comes back tight is worth noticing rather than
// mistaking for a fix.
//
// The first boot is discarded rather than counted. A node's flash persists
// between runs exactly as a board's does, so the first one formats a filesystem
// and generates an identity and the ones after it do not; measuring it against
// them would compare two different pieces of work.
//
//	MESHBENCH_LIVE=1 go test ./internal/sim/engine/ \
//	  -run TestLiveEmulatedTimingIsNotReproducible -v -timeout 1200s
func TestLiveEmulatedTimingIsNotReproducible(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}
	if os.Getenv(emulated.EnvQEMU) == "" {
		if _, err := exec.LookPath("qemu-system-xtensa"); err != nil {
			if _, err := os.Stat(qemuInToolsDir()); err != nil {
				t.Skipf("no emulator: set %s to a build carrying the SX1262 device",
					emulated.EnvQEMU)
			}
		}
	}

	const seed = 4417
	spec := emulatedRepeater()
	if why := scenario.NotReproducible([]scenario.Node{spec}); why == "" {
		t.Fatal("a scenario with an emulated node reported itself reproducible; " +
			"every timing below is then quoted as though it could be repeated")
	} else {
		t.Log(why)
	}

	t.Log("warm-up boot, discarded")
	if _, err := firstTransmissionMs(t, spec, seed); err != nil {
		t.Fatalf("warm-up: %v", err)
	}

	var at []uint32
	for run := 1; run <= 3; run++ {
		ms, err := firstTransmissionMs(t, spec, seed)
		if err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		t.Logf("run %d of seed %d: first transmission at %d ms", run, seed, ms)
		at = append(at, ms)
	}
	if at[0] == at[1] && at[1] == at[2] {
		t.Logf("three runs of seed %d all put it at %d ms; that is one draw "+
			"agreeing with itself, not a guarantee", seed, at[0])
		return
	}
	lo, hi := at[0], at[0]
	for _, ms := range at {
		if ms < lo {
			lo = ms
		}
		if ms > hi {
			hi = ms
		}
	}
	t.Logf("three runs of seed %d, spread %d ms: %v", seed, hi-lo, at)
}

// firstTransmissionMs boots the node, runs it until it says something, and
// answers with the simulated instant it said it at.
//
// One step per tick against the wall, which is what the workbench itself does
// and is the whole reason the resolution is worth arguing about. An emulated
// node cannot be run faster than it runs, so the engine has to be held to
// roughly real time whatever it is stepping; but a coarse pace also quantises
// the answer. Stepping half a second at a time and then sleeping through it
// stamps every frame that arrives during the sleep with the first instant of
// the next stretch, so two runs that differ by a quarter of a second report the
// same millisecond and the spread being measured disappears into the harness.
func firstTransmissionMs(t *testing.T, spec scenario.Node, seed uint64) (uint32, error) {
	t.Helper()
	const stepMs = 10
	e := engine.New(flat{100}, engine.Config{
		FreqMHz: 869.618, SF: 8, BandwidthHz: 62_500, CodingRate: 4,
		NoiseFigDB: 6, StepMs: stepMs, Seed: seed,
	})
	defer func() { _ = e.Close() }()
	e.Add(spec, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Second)
	defer cancel()
	if err := e.AttachNative(ctx, seed); err != nil {
		return 0, err
	}
	tick := time.NewTicker(stepMs * time.Millisecond)
	defer tick.Stop()
	deadline := time.Now().Add(240 * time.Second)
	for time.Now().Before(deadline) {
		<-tick.C
		if err := e.Step(ctx); err != nil {
			return 0, err
		}
		for _, ev := range e.Events() {
			if ev.Kind == "tx" && ev.From == spec.Name {
				return ev.AtMs, nil
			}
		}
	}
	return 0, errNothingHeard
}

var errNothingHeard = errors.New("the emulated node never transmitted")

// emulatedRepeater is the node under measurement: one published image, on a
// board whose wiring somebody has watched boot.
func emulatedRepeater() scenario.Node {
	return scenario.Node{
		Name: "timing-e22", Kind: scenario.SimpleRepeater,
		Position: scenario.LatLon{Lat: 56.70, Lon: -3.90}, HeightAGLm: 10,
		Antenna: antenna.Mounted{
			Pattern: antenna.Collinear{GainDBiPeak: 6}, Polarisation: "vertical"},
		TxPowerDBm: 20, NoiseFigureDB: 6,
		Radio: scenario.RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62_500,
			SpreadFactor: 8, CodingRate: 4},
		Firmware: scenario.FirmwareRef{
			Role: "simple_repeater", Version: "v1.17.0",
			Board: "Generic_E22_sx1262",
		},
	}
}

// qemuInToolsDir is where a fetched toolchain puts the emulator, which is the
// lookup a boot actually does - so a machine set up entirely by resource.fetch
// runs this rather than skipping it for want of a PATH entry it never needed.
func qemuInToolsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home + "/.cache/meshbench/tools/qemu-system-xtensa"
}
