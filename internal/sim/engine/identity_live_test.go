package engine_test

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/firmware/emulated"
	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Two emulated nodes, and they must not be the same node twice.
//
// Every emulated board used to come up with the same keypair, byte for byte,
// across boards and across runs. A mesh of them was one node repeated: every
// packet signed by the same identity, every receiver treating them as one peer,
// and anything keyed on identity - contacts, ACK routing, flood suppression -
// operating on a mesh that did not exist. Their adverts were byte-identical too,
// so a receiver was right to drop the second as a duplicate.
//
// The cause was entropy. Since v1.17.1 MeshCore mixes the radio's own randomness
// into its keypair, and `SX126x::randomByte()` derives that from RSSI noise on a
// receiving radio. Our chip had no noise to give, so three deterministic sources
// xored together stayed deterministic. The chip has per-node receiver noise now,
// seeded from the run's seed and the node's name.
//
// This is the assertion the fix is actually about, and the one nothing else
// makes. `noiseSeedFor` is unit-tested to give distinct seeds per name, and the
// chip model is tested to give distinct noise per seed, but the link from noise
// to the key the firmware derives runs through real firmware and can only be
// seen by running two of them.
//
// Determinism is not traded away, and that is a separate claim this does not
// assert: checking it needs two runs, and pinning the expected keys here would
// break on any MeshCore version rather than on a fault of ours. It was checked
// by hand when this was written - two runs of this test, same seed, same names,
// byte-identical keys both times - and the seed is derived rather than sampled
// precisely so that stays true.
//
//	MESHBENCH_LIVE=1 \
//	MESHBENCH_QEMU=~/msim/espqemu-src/build/qemu-system-xtensa \
//	MESHBENCH_RADIO_LIB=~/…/virtual-sx1262/build/libvirtualsx1262.so \
//	go test ./internal/sim/engine -run TestTwoEmulatedNodesAreTwoNodes -v -timeout 600s
func TestTwoEmulatedNodesAreTwoNodes(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}
	const board = "Generic_E22_sx1262"
	const version = "v1.17.1"

	cache := firmware.DefaultCacheDir()
	img := emulated.BoardImage{Board: board, Role: "simple_repeater",
		Version: version, Format: "bin"}
	if _, err := os.Stat(emulated.BoardImagePath(cache, img)); err != nil {
		t.Skipf("no cached %s image for %s; probe that board first", board, version)
	}

	keys := identitiesOfTwoEmulatedNodes(t, board, version)
	if len(keys) != 2 {
		t.Fatalf("only %d of 2 nodes printed an identity: %v", len(keys), keys)
	}
	if strings.EqualFold(keys["emu-a"], keys["emu-b"]) {
		t.Errorf("both emulated nodes came up as %s...; a mesh of them is one "+
			"node repeated", keys["emu-a"][:16])
	}
	t.Logf("emu-a %s", keys["emu-a"])
	t.Logf("emu-b %s", keys["emu-b"])
}

// repeaterID is what simple_repeater prints as it comes up.
var repeaterID = regexp.MustCompile(`Repeater ID: ([0-9A-Fa-f]{64})`)

func identitiesOfTwoEmulatedNodes(t *testing.T, board, version string) map[string]string {
	t.Helper()

	e := engine.New(flat{}, engine.Config{
		FreqMHz: 869.618, SF: 8, BandwidthHz: 62_500, CodingRate: 4,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417,
	})
	defer func() { _ = e.Close() }()

	mast := antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6}, Polarisation: "vertical"}
	radio := scenario.RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62_500,
		SpreadFactor: 8, CodingRate: 4}
	for i, name := range []string{"emu-a", "emu-b"} {
		e.Add(scenario.Node{
			Name: name, Kind: scenario.SimpleRepeater,
			Position:   scenario.LatLon{Lat: 56.70, Lon: -3.90 + float64(i)*0.02},
			HeightAGLm: 10,
			Antenna:    mast, TxPowerDBm: 20, NoiseFigureDB: 6, Radio: radio,
			Firmware: scenario.FirmwareRef{
				Role: "simple_repeater", Version: version, Board: board,
			},
		}, nil)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 480*time.Second)
	defer cancel()

	if err := e.AttachNative(ctx, 4417); err != nil {
		t.Fatal(err)
	}
	t.Log("both emulated nodes attached")

	// Paced to wall time: these are emulators and cannot be run faster than they
	// run. Long enough for both to come up and print, and no longer.
	deadline := time.Now().Add(90 * time.Second)
	for at := uint32(500); time.Now().Before(deadline); at += 500 {
		if err := e.Run(ctx, at); err != nil {
			t.Fatalf("run to %d ms: %v", at, err)
		}
		time.Sleep(450 * time.Millisecond)
	}

	// Read from each node's own console log, the way the board probe does. The
	// identity is printed once, in the first seconds of the boot, so the whole
	// log is the only place it is still there to be read afterwards.
	out := map[string]string{}
	for _, name := range []string{"emu-a", "emu-b"} {
		n, ok := e.NodeByName(name)
		if !ok || n.Firmware == nil {
			t.Fatalf("%s did not attach", name)
		}
		said, ok := n.Firmware.Backend.(interface{ ConsoleLog() ([]byte, error) })
		if !ok {
			t.Fatalf("%s has no console to read", name)
		}
		log, err := said.ConsoleLog()
		if err != nil {
			t.Fatalf("%s console: %v", name, err)
		}
		if m := repeaterID.FindSubmatch(log); m != nil {
			out[name] = string(m[1])
		}
	}
	return out
}
