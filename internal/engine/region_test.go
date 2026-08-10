package engine_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/A13xB0/meshcoresim/internal/antenna"
	"github.com/A13xB0/meshcoresim/internal/engine"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// Regions are the firmware's business, not the channel's.
//
// The division this asserts: the RF layer delivers a packet to a node whether
// or not that node will accept it, and the *firmware* is what refuses. If the
// simulator filtered by region itself it would be enforcing a policy rather
// than observing one — and a bug in MeshCore's own region handling would become
// invisible, which is the single class of bug this project exists to find.
//
// So: deny flooding on one node via its own CLI, and require that the channel
// still delivered while that node stopped relaying.
func TestFirmwareRejectsByRegionAndTheChannelDoesNot(t *testing.T) {
	if os.Getenv("MESHCORESIM_LIVE") == "" {
		t.Skip("set MESHCORESIM_LIVE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	e := engine.New(flat{100}, engine.Config{
		FreqMHz: 869.618, SF: 8, BandwidthHz: 62500, CodingRate: 4,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417,
	})
	defer func() { _ = e.Close() }()

	mast := antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6}, Polarisation: "vertical"}
	for _, spec := range []struct {
		name string
		lon  float64
	}{{"alpha", -3.90}, {"bravo", -3.55}, {"charlie", -3.20}} {
		e.Add(scenario.Node{
			Name: spec.name, Kind: scenario.SimpleRepeater,
			Position: scenario.LatLon{Lat: 56.70, Lon: spec.lon}, HeightAGLm: 10,
			Antenna: mast, TxPowerDBm: 10, NoiseFigureDB: 6,
			Radio: scenario.RadioConfig{
				CentreHz: 869.618e6, BandwidthHz: 62500, SpreadFactor: 8, CodingRate: 4},
		}, nil)
	}
	if err := e.AttachNative(ctx, 4417); err != nil {
		t.Fatal(err)
	}

	// bravo refuses to flood — its own decision, made by its own region map.
	bravo, _ := e.NodeByName("bravo")
	for _, cmd := range []string{"region denyf *", "region save"} {
		if err := bravo.Firmware.Bridge.Type([]byte(cmd + "\r\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.Run(ctx, 4000); err != nil {
		t.Fatal(err)
	}

	alpha, _ := e.NodeByName("alpha")
	if err := alpha.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(ctx, 30_000); err != nil {
		t.Fatal(err)
	}

	deliveredToBravo, bravoRelayed := 0, 0
	for _, ev := range e.Events() {
		switch {
		case ev.Kind == "rx" && ev.To == "bravo":
			deliveredToBravo++
		case ev.Kind == "tx" && ev.From == "bravo":
			bravoRelayed++
		}
	}
	t.Logf("delivered to bravo: %d, relayed by bravo: %d", deliveredToBravo, bravoRelayed)

	// The channel's job, done regardless of what the firmware will do with it.
	if deliveredToBravo == 0 {
		t.Error("the channel withheld a packet from a node in range; region policy " +
			"has leaked into the RF layer")
	}
	// The firmware's job. Its own adverts still go out, so this is about the
	// relay specifically.
	if bravoRelayed > 1 {
		t.Errorf("bravo relayed %d times while denying floods; the firmware's own "+
			"region decision was not respected", bravoRelayed)
	}
}

// A guard against the coupling creeping back: the engine must not import the
// region machinery at all.
func TestEngineDoesNotReferenceRegions(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Skip(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") ||
			strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(e.Name())
		if err != nil {
			continue
		}
		for _, forbidden := range []string{"scenario.Region", "Participates(", ".Contains("} {
			if strings.Contains(string(b), forbidden) {
				t.Errorf("%s references %s — regions belong to the firmware, and a channel "+
					"that filters by them hides the firmware's own bugs", e.Name(), forbidden)
			}
		}
	}
}
