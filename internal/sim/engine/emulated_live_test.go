package engine_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// One emulated node and one native node on the same channel.
//
// This is the whole point of having two backends. The native node runs MeshCore
// compiled for this machine; the emulated one runs the image people flash,
// unmodified, inside an emulator. They share one RF engine, so whatever crosses
// between them crossed a simulated radio channel rather than a function call.
//
// What it asserts is deliberately modest: that the emulated node transmits, and
// that the native node hears it. A frame arriving is the thing that was in doubt
// for the whole of this exercise, and it is not in doubt once a receiver has
// decoded one.
//
//	MESHBENCH_LIVE=1 \
//	MESHBENCH_QEMU=~/msim/espqemu-src/build/qemu-system-xtensa \
//	MESHBENCH_RADIO_SERVER=/tmp/radioserver \
//	go test ./internal/engine/ -run TestEmulatedAndNativeShareAChannel -v -timeout 400s
func TestEmulatedAndNativeShareAChannel(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}
	const board = "Generic_E22_sx1262"
	const version = "v1.17.0"

	// The image has to be in the cache: the engine will not reach for the
	// network mid-attach, because a scenario that quietly downloads a hundred
	// megabytes when someone presses play is not one anybody trusts.
	cache := firmware.DefaultCacheDir()
	img := firmware.BoardImage{Board: board, Role: "simple_repeater",
		Version: version, Format: "bin"}
	if _, err := os.Stat(firmware.BoardImagePath(cache, img)); err != nil {
		bc := &firmware.BoardCatalogue{CacheDir: cache}
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		all, err := bc.ListAll(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var want firmware.BoardImage
		for _, i := range firmware.Runnable(all, hw.EmulationSupported) {
			if i.Board == board && i.Version == version && i.Role == "simple_repeater" {
				want = i
			}
		}
		if want.Name == "" {
			t.Fatalf("no published %s image for %s", board, version)
		}
		t.Logf("fetching %s", want.Name)
		if _, err := bc.Ensure(ctx, want); err != nil {
			t.Fatal(err)
		}
	}

	e := engine.New(flat{}, engine.Config{
		FreqMHz: 869.618, SF: 8, BandwidthHz: 62_500, CodingRate: 4,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417,
	})
	defer func() { _ = e.Close() }()

	// Close together and well above the demodulator floor: the question here is
	// whether a frame crosses the backends at all, and a delivery that depends
	// on a decibel would make the answer a coin toss.
	mast := antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6}, Polarisation: "vertical"}
	radio := scenario.RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62_500,
		SpreadFactor: 8, CodingRate: 4}

	e.Add(scenario.Node{
		Name: "emulated-e22", Kind: scenario.SimpleRepeater,
		Position: scenario.LatLon{Lat: 56.70, Lon: -3.90}, HeightAGLm: 10,
		Antenna: mast, TxPowerDBm: 20, NoiseFigureDB: 6, Radio: radio,
		Firmware: scenario.FirmwareRef{
			Role: "simple_repeater", Version: version, Board: board,
		},
	}, nil)
	e.Add(scenario.Node{
		Name: "native-listener", Kind: scenario.SimpleRepeater,
		Position: scenario.LatLon{Lat: 56.70, Lon: -3.88}, HeightAGLm: 10,
		Antenna: mast, TxPowerDBm: 20, NoiseFigureDB: 6, Radio: radio,
		Firmware: scenario.FirmwareRef{
			Role: "simple_repeater", Version: "repeater-v1.17.0",
		},
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	if err := e.AttachNative(ctx, 4417); err != nil {
		t.Fatal(err)
	}
	t.Log("both nodes attached")

	// Paced to wall time, because one of these nodes is an emulator and cannot
	// be run faster than it runs. A native-only scenario would race through
	// this in milliseconds.
	deadline := time.Now().Add(60 * time.Second)
	for at := uint32(500); time.Now().Before(deadline); at += 500 {
		if err := e.Run(ctx, at); err != nil {
			t.Fatalf("run to %d ms: %v", at, err)
		}
		time.Sleep(450 * time.Millisecond)
	}

	tx, rx := map[string]int{}, map[string]int{}
	for _, ev := range e.Events() {
		switch ev.Kind {
		case "tx":
			tx[ev.From]++
		case "rx":
			rx[ev.To]++
			t.Logf("%s heard %s at %d ms: %s", ev.To, ev.From, ev.AtMs, ev.Detail)
		}
	}
	t.Logf("transmissions %v", tx)
	t.Logf("receptions    %v", rx)

	if tx["emulated-e22"] == 0 {
		t.Error("the emulated node never transmitted")
	}
	if rx["native-listener"] == 0 {
		t.Error("the native node never heard the emulated one; " +
			"the two backends are not sharing a channel")
	}
}
