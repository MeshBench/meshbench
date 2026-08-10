package engine_test

import (
	"context"
	"os"
	"testing"

	"github.com/A13xB0/meshcoresim/internal/antenna"
	"github.com/A13xB0/meshcoresim/internal/engine"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// The regression this exists to hold: a flood must cross more than one hop,
// and a node must be able to transmit more than once in its life.
//
// Two bugs each independently reduced every flood to a single hop. The engine
// never sent TransmitFinished, so each node's radio wedged after its first
// packet; and every node shared one working directory, so they all loaded the
// first node's identity and dropped each other's packets as their own echoes.
// Both produced exactly the same symptom from the outside, which is why this
// asserts on the mechanism — repeated transmission and cross-node relay — and
// not just on delivery counts.
//
// Node names are prefixed per test file. Node storage is keyed by name, so a
// test that saved "region denyf *" at a node called bravo silently configured
// every later test's bravo — which is exactly what happened.
func TestLiveFloodCrossesHopsAndNodesTransmitTwice(t *testing.T) {
	if os.Getenv("MESHCORESIM_LIVE") == "" {
		t.Skip("set MESHCORESIM_LIVE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120e9)
	defer cancel()

	e := engine.New(flat{}, engine.Config{
		FreqMHz: 869.525, SF: 10, BandwidthHz: 250e3, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417,
	})
	defer func() { _ = e.Close() }()

	// A chain: A hears B, B hears C, but A cannot reach C directly — so a
	// packet at C proves a relay happened, not just a loud transmitter.
	mast := antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6}, Polarisation: "vertical"}
	for i, spec := range []struct {
		name     string
		lat, lon float64
	}{
		// ~37 km hops: comfortably decodable (SNR about -4 dB at SF10), while
		// the 73 km end-to-end path is far below the demodulator floor. The
		// original spacing put alpha->charlie at 142.9 dB — marginally
		// decodable — so charlie heard alpha directly, relayed first, and
		// MeshCore's own redundancy suppression cancelled bravo's relay: the
		// test failed because its geometry premise was false, not the flood.
		{"fld-alpha", 56.70, -3.90},
		{"fld-bravo", 56.70, -3.30},
		{"fld-charlie", 56.70, -2.70},
	} {
		_ = i
		e.Add(scenario.Node{
			Name: spec.name, Kind: scenario.SimpleRepeater,
			Position: scenario.LatLon{Lat: spec.lat, Lon: spec.lon}, HeightAGLm: 10,
			Antenna: mast, TxPowerDBm: 10, NoiseFigureDB: 6,
			Radio: scenario.RadioConfig{CentreHz: 869.525e6, BandwidthHz: 250e3, SpreadFactor: 10, CodingRate: 1},
		}, nil)
	}

	if err := e.AttachNative(ctx, 4417); err != nil {
		t.Fatal(err)
	}

	// The workbench's own path: type at the source's real CLI.
	node, ok := e.NodeByName("fld-alpha")
	if !ok || node.Firmware == nil {
		t.Fatal("alpha has no firmware")
	}
	if err := node.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(ctx, 20_000); err != nil {
		t.Fatal(err)
	}
	// A second command later proves the radio is not wedged after the first
	// transmission — the precise failure TransmitFinished's absence caused.
	if err := node.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(ctx, 40_000); err != nil {
		t.Fatal(err)
	}

	tx := map[string]int{}
	rxAt := map[string]int{}
	for _, ev := range e.Events() {
		switch ev.Kind {
		case "tx":
			tx[ev.From]++
		case "rx":
			rxAt[ev.To]++
		}
	}
	t.Logf("tx=%v rxAt=%v", tx, rxAt)

	if tx["fld-alpha"] < 2 {
		t.Errorf("alpha transmitted %d times; a radio that cannot send twice is wedged", tx["fld-alpha"])
	}
	if tx["fld-bravo"] == 0 {
		t.Error("bravo relayed nothing; the flood died at the first hop")
	}
	if rxAt["fld-charlie"] == 0 {
		t.Error("charlie heard nothing; it is out of alpha's direct reach, so a relay was the only way")
	}
}
