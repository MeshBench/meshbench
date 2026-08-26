package engine_test

import (
	"context"
	"os"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A single room server, on its own, doing nothing.
//
// Kept because the three-role test could not tell a room server that hangs from
// a room server that simply does not forward: both look like silence at the far
// end. This one asks the narrower question, so a stall here says the firmware
// stopped and a stall there says the geometry is wrong.
func TestLiveRoomServerKeepsTicking(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120e9)
	defer cancel()

	e := engine.New(flat{}, engine.Config{
		FreqMHz: 869.525, SF: 10, BandwidthHz: 250e3, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417,
	})
	defer func() { _ = e.Close() }()

	e.Add(scenario.Node{
		Name: "probe-room", Kind: scenario.RoomServer,
		Position: scenario.LatLon{Lat: 56.70, Lon: -3.90}, HeightAGLm: 10,
		Antenna:    antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6}, Polarisation: "vertical"},
		TxPowerDBm: 10, NoiseFigureDB: 6,
		Radio: scenario.RadioConfig{CentreHz: 869.525e6, BandwidthHz: 250e3,
			SpreadFactor: 10, CodingRate: 1},
		Firmware: scenario.FirmwareRef{Version: "room-server-v1.17.0"},
	}, nil)

	if err := e.AttachNative(ctx, 4417); err != nil {
		t.Fatal(err)
	}
	t.Log("attached")

	for _, at := range []uint32{2_000, 6_000, 10_000, 20_000, 40_000} {
		if err := e.Run(ctx, at); err != nil {
			t.Fatalf("stalled before %d ms: %v", at, err)
		}
		t.Logf("reached %d ms", at)
	}

	node, _ := e.NodeByName("probe-room")
	if err := node.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(ctx, 60_000); err != nil {
		t.Fatalf("stalled after advert: %v", err)
	}
	tx := 0
	for _, ev := range e.Events() {
		if ev.Kind == "tx" {
			tx++
		}
	}
	t.Logf("transmissions: %d", tx)
	if tx == 0 {
		t.Error("a room server that will not advert has no way to be discovered")
	}
}
