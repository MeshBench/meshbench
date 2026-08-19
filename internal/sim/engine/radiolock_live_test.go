package engine_test

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Locking, which is the failure this codebase actually has.
//
// Three separate freezes have been traced to a wait with no bound: firmware
// start, sim.run, and a boot-offset Advance on a node whose process had gone.
// Each presented identically - a window that stopped drawing and a control
// socket that stopped answering - and each took a goroutine dump to tell apart
// from a crash.
//
// The virtual SX1262 planned in docs/virtual-sx1262.md makes that risk worse
// before it makes it better: RadioLib spins on a BUSY line, and a spin that
// does not advance simulated time never ends. So these tests exist to fail
// loudly and quickly rather than to hang a suite.
//
// They are all written the same way: run the work in a goroutine, wait on a
// timer, and report a stack dump if the timer wins. A test that hangs tells you
// nothing; a test that says "this is where it stopped" tells you everything.

// A tick must always finish, even under heavy contention.
//
// Twelve nodes on top of each other, all transmitting: every node is deaf while
// keyed, every reception collides, and the demodulator is doing real work on
// every pair. If any lock ordering in the engine is wrong, this is where it
// shows.
func TestRadioStackDoesNotDeadlockUnderContention(t *testing.T) {
	if os.Getenv("MESHCORESIM_LIVE") == "" {
		t.Skip("set MESHCORESIM_LIVE=1")
	}
	const nodes = 12
	e := engine.New(flat{}, engine.Config{
		FreqMHz: 869.525, SF: 10, BandwidthHz: 250e3, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417,
	})
	defer func() { _ = e.Close() }()

	mast := antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6}, Polarisation: "vertical"}
	for i := 0; i < nodes; i++ {
		e.Add(scenario.Node{
			Name: "lk-" + string(rune('a'+i)), Kind: scenario.SimpleRepeater,
			// Deliberately close: everyone hears everyone, so every
			// transmission contends with every other.
			Position:   scenario.LatLon{Lat: 56.70, Lon: -3.90 + float64(i)*0.01},
			HeightAGLm: 10, Antenna: mast, TxPowerDBm: 10, NoiseFigureDB: 6,
			Radio: scenario.RadioConfig{
				CentreHz: 869.525e6, BandwidthHz: 250e3, SpreadFactor: 10, CodingRate: 1,
			},
			// Pinned, not "main": a golden file recorded against a moving
			// target is worth nothing, and the whole point of this test is to
			// compare one radio stack against another with everything else
			// held still.
			Firmware: scenario.FirmwareRef{Role: "simple_repeater", Version: goldenFirmware},
		}, nil)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	if err := e.AttachNative(ctx, 4417); err != nil {
		t.Fatal(err)
	}

	// Everyone talks at once, repeatedly. One advert each is not contention.
	for round := 0; round < 3; round++ {
		for i := 0; i < nodes; i++ {
			n, ok := e.NodeByName("lk-" + string(rune('a'+i)))
			if !ok || n.Firmware == nil {
				continue
			}
			_ = n.Firmware.Bridge.Type([]byte("advert\r\n"))
		}
		mustFinish(t, 90*time.Second, "run round", func() error {
			return e.Run(ctx, uint32(20_000*(round+1)))
		})
	}

	tx := 0
	for _, ev := range e.Events() {
		if ev.Kind == "tx" {
			tx++
		}
	}
	if tx < nodes {
		t.Errorf("%d transmissions from %d nodes; under contention every node should still get on the air",
			tx, nodes)
	}
}

// A node whose firmware has gone must not stall the engine.
//
// This is the exact shape of the freeze that took an evening: waitAttached had
// a bound, the boot-offset Advance that followed it did not, and a process that
// died in between left the frame thread waiting for an acknowledgement that
// could never arrive. Closing the bridge must make every wait on it fail, not
// block.
func TestEngineSurvivesFirmwareVanishing(t *testing.T) {
	if os.Getenv("MESHCORESIM_LIVE") == "" {
		t.Skip("set MESHCORESIM_LIVE=1")
	}
	e := engine.New(flat{}, engine.Config{
		FreqMHz: 869.525, SF: 10, BandwidthHz: 250e3, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417,
	})
	defer func() { _ = e.Close() }()

	mast := antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6}, Polarisation: "vertical"}
	for _, lon := range []float64{-3.90, -3.70} {
		e.Add(scenario.Node{
			Name: "vn-" + string(rune('a'+int((lon+4)*10))), Kind: scenario.SimpleRepeater,
			Position: scenario.LatLon{Lat: 56.70, Lon: lon}, HeightAGLm: 10,
			Antenna: mast, TxPowerDBm: 10, NoiseFigureDB: 6,
			Radio: scenario.RadioConfig{
				CentreHz: 869.525e6, BandwidthHz: 250e3, SpreadFactor: 10, CodingRate: 1,
			},
			// Pinned, not "main": a golden file recorded against a moving
			// target is worth nothing, and the whole point of this test is to
			// compare one radio stack against another with everything else
			// held still.
			Firmware: scenario.FirmwareRef{Role: "simple_repeater", Version: goldenFirmware},
		}, nil)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := e.AttachNative(ctx, 4417); err != nil {
		t.Fatal(err)
	}

	// Take one node's firmware away mid-run, as a crash would.
	for _, n := range e.Nodes() {
		if n.Firmware != nil {
			_ = n.Firmware.Bridge.Close()
			break
		}
	}

	// The engine must notice and carry on - or fail - inside a bounded time.
	// What it must not do is wait for ever.
	mustFinish(t, 60*time.Second, "step with a dead node", func() error {
		_ = e.Run(ctx, 5_000)
		return nil
	})
}

// mustFinish runs work with a deadline and, if it misses, prints every
// goroutine's stack before failing.
//
// A hung test that simply times out tells you it hung. This tells you where,
// which is the difference between a five-minute diagnosis and an evening -
// exactly the evening the boot-offset Advance cost.
func mustFinish(t *testing.T, within time.Duration, what string, fn func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		if err != nil && !strings.Contains(err.Error(), "context") {
			t.Fatalf("%s: %v", what, err)
		}
	case <-time.After(within):
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		t.Fatalf("%s did not finish within %s - this is a lock, not slowness.\n\n%s",
			what, within, buf[:n])
	}
}
