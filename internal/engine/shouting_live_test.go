package engine_test

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/antenna"
	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// How far does a generic E22 dev board shout, when nothing is in the way?
//
// One emulated node running the published Generic_E22_sx1262 v1.17.0 image -
// the bytes off the flasher, unmodified - and a fan of native listeners
// marching east over flat ground. Nothing here is realistic: no terrain, no
// buildings, no curvature, no multipath. That is the point. It puts a ceiling
// on the answer, and the ceiling is the only number a simulator this kind can
// honestly claim.
//
//	MESHCORESIM_LIVE=1 MESHCORESIM_QEMU=... MESHCORESIM_RADIO_SERVER=... \
//	go test ./internal/engine/ -run TestHowFarDoesADevBoardShout -v -timeout 400s
func TestHowFarDoesADevBoardShout(t *testing.T) {
	if os.Getenv("MESHCORESIM_LIVE") == "" {
		t.Skip("set MESHCORESIM_LIVE=1")
	}

	// EU/UK (Narrow) - what ScotMesh actually runs.
	const (
		freqMHz = 869.618
		bwHz    = 62_500
		sf      = 8
		cr      = 4
	)
	radio := scenario.RadioConfig{CentreHz: freqMHz * 1e6, BandwidthHz: bwHz,
		SpreadFactor: sf, CodingRate: cr}
	mast := antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6},
		Polarisation: "vertical"}

	e := engine.New(flat{}, engine.Config{
		FreqMHz: freqMHz, SF: sf, BandwidthHz: bwHz, CodingRate: cr,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417,
	})
	defer func() { _ = e.Close() }()

	const originLat, originLon = 56.70, -3.90
	// One degree of longitude, in km, at this latitude - so a distance in km
	// can be placed without a projection.
	kmPerDegLon := 111.320 * math.Cos(originLat*math.Pi/180)

	e.Add(scenario.Node{
		Name: "shout-e22", Kind: scenario.SimpleRepeater,
		Position: scenario.LatLon{Lat: originLat, Lon: originLon}, HeightAGLm: 10,
		Antenna: mast, TxPowerDBm: 22, NoiseFigureDB: 6, Radio: radio,
		Firmware: scenario.FirmwareRef{
			Role: "simple_repeater", Version: "v1.17.0",
			Board: "Generic_E22_sx1262",
		},
	}, nil)

	distances := []float64{25, 50, 100, 200, 400, 800}
	name := func(km float64) string { return fmt.Sprintf("ear-%.0fkm", km) }
	for _, km := range distances {
		e.Add(scenario.Node{
			Name: name(km), Kind: scenario.SimpleRepeater,
			Position:   scenario.LatLon{Lat: originLat, Lon: originLon + km/kmPerDegLon},
			HeightAGLm: 10, Antenna: mast, TxPowerDBm: 22, NoiseFigureDB: 6,
			Radio:    radio,
			Firmware: scenario.FirmwareRef{Version: "repeater-v1.17.0"},
		}, nil)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	if err := e.AttachNative(ctx, 4417); err != nil {
		t.Fatal(err)
	}
	t.Logf("1 emulated board + %d native ears, all listening", len(distances))

	// Paced 1:1, because one of these is an emulator and cannot be hurried.
	// The board adverts on its own; nobody types anything at it.
	deadline := time.Now().Add(75 * time.Second)
	for at := uint32(500); time.Now().Before(deadline); at += 500 {
		if err := e.Run(ctx, at); err != nil {
			t.Fatalf("run to %d ms: %v", at, err)
		}
		time.Sleep(450 * time.Millisecond)
	}

	type ear struct {
		km      float64
		heard   bool
		snr     float64
		outcome string
	}
	best := map[string]*ear{}
	for _, km := range distances {
		best[name(km)] = &ear{km: km}
	}
	txs := 0
	for _, ev := range e.Events() {
		if ev.Kind == "tx" && ev.From == "shout-e22" {
			txs++
			continue
		}
		got, ok := best[ev.To]
		if !ok || ev.From != "shout-e22" {
			continue
		}
		switch ev.Kind {
		case "rx":
			if !got.heard || ev.SNRdB > got.snr {
				got.heard, got.snr, got.outcome = true, ev.SNRdB, string(ev.Outcome)
			}
		case "miss":
			if !got.heard {
				got.snr, got.outcome = ev.SNRdB, string(ev.Outcome)
			}
		}
	}

	out := make([]*ear, 0, len(best))
	for _, v := range best {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].km < out[j].km })

	t.Logf("the board transmitted %d time(s)", txs)
	t.Log("  distance     heard   SNR      outcome")
	for _, v := range out {
		mark := "no"
		if v.heard {
			mark = "yes"
		}
		t.Logf("  %6.0f km    %-5s  %6.1f dB  %s", v.km, mark, v.snr, v.outcome)
	}

	if txs == 0 {
		t.Fatal("the emulated board never transmitted, so there is nothing to measure")
	}
	if !out[0].heard {
		t.Errorf("nothing heard it at %.0f km, which is too close to be a range limit",
			out[0].km)
	}
	// A ladder with no top is not a range measurement, it is a link budget with
	// the losses left out.
	if out[len(out)-1].heard {
		t.Errorf("still heard at %.0f km; extend the ladder or the geometry is wrong",
			out[len(out)-1].km)
	}
}
