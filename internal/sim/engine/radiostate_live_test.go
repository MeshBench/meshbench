package engine_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// What a published image actually does to its radio, watched from below.
//
// The two faults MeshCore 1.17.1 fixed are both invisible from outside the chip:
// receive gain reverting after an AGC reset, and a transmit-enable line that
// never goes high. Neither logs anything, and the firmware's own CLI goes on
// reporting the setting the operator chose. This runs the real image and records
// every change the radio's configuration goes through, which is the only place
// either fault shows.
//
//	MESHBENCH_LIVE=1 \
//	MESHBENCH_QEMU=~/msim/espqemu-src/build/qemu-system-xtensa \
//	MESHBENCH_RADIO_SERVER=~/.cache/meshbench/tools/radioserver \
//	go test ./internal/engine/ -run TestTheRadioReportsHowTheFirmwareConfiguredIt -v -timeout 400s
func TestTheRadioReportsHowTheFirmwareConfiguredIt(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}
	const board = "Generic_E22_sx1262"
	const version = "v1.17.0"

	cache := firmware.DefaultCacheDir()
	img := firmware.BoardImage{Board: board, Role: "simple_repeater",
		Version: version, Format: "bin"}
	if _, err := os.Stat(firmware.BoardImagePath(cache, img)); err != nil {
		t.Skipf("no cached image for %s %s", board, version)
	}

	e := engine.New(flat{}, engine.Config{
		FreqMHz: 869.618, SF: 8, BandwidthHz: 62_500, CodingRate: 4,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417,
	})
	defer func() { _ = e.Close() }()

	mast := antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6},
		Polarisation: "vertical"}
	radio := scenario.RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62_500,
		SpreadFactor: 8, CodingRate: 4}

	e.Add(scenario.Node{
		Name: "e22", Kind: scenario.SimpleRepeater,
		Position: scenario.LatLon{Lat: 56.70, Lon: -3.90}, HeightAGLm: 10,
		Antenna: mast, TxPowerDBm: 22, NoiseFigureDB: 6, Radio: radio,
		FEM: &hw.FEM{TxGainDB: 0, TxLossDB: 25},
		Firmware: scenario.FirmwareRef{
			Role: "simple_repeater", Version: version, Board: board,
		},
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	if err := e.AttachNative(ctx, 4417); err != nil {
		t.Fatal(err)
	}
	t.Log("attached")

	node := e.Nodes()[0]
	var last string
	var reported bool

	// Paced to wall time: one of these is an emulator and cannot be hurried.
	deadline := time.Now().Add(75 * time.Second)
	for at := uint32(500); time.Now().Before(deadline); at += 500 {
		if err := e.Run(ctx, at); err != nil {
			t.Fatalf("run to %d ms: %v", at, err)
		}
		st := node.Firmware.Bridge.Stats()
		if !st.Configured {
			time.Sleep(400 * time.Millisecond)
			continue
		}
		reported = true
		// Only the changes. A line per tick would bury the four moments that
		// matter in seven thousand that repeat.
		now := fmt.Sprintf("gain=%#02x boosted=%v tx=%d fem=%v femAtTx=%d mode=%d "+
			"sf=%d bw=%d freq=%d preamble=%d irqmask=%#04x",
			st.RxGainReg, st.RxBoosted(), st.TxPowerDBm, st.FemEnabled,
			st.FemAtTx, st.Mode, st.SF, st.BandwidthHz, st.FreqHz,
			st.PreambleSyms, st.IRQMask)
		if now != last {
			t.Logf("%6d ms  %s", at, now)
			last = now
		}
		time.Sleep(400 * time.Millisecond)
	}

	if !reported {
		t.Fatal("the radio never reported its configuration")
	}
	t.Logf("engine's effective figures: tx=%.1f dBm noise figure=%.1f dB",
		node.Spec.TxPowerDBm, node.Spec.NoiseFigureDB)
}
