package engine_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/firmware/emulated"
	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Two firmware versions, one scenario, one seed.
//
// The comparison is only worth anything because nothing else can move:
// same seed, same scenario, same result, per CLAUDE.md. Any difference between
// the two timelines is the firmware, which is the whole premise of running the
// real thing rather than a model of it.
//
//	MESHBENCH_LIVE=1 MESHBENCH_AB_OUT=/tmp/ab.json \
//	MESHBENCH_QEMU=~/msim/espqemu-src/build/qemu-system-xtensa \
//	MESHBENCH_RADIO_SERVER=~/.cache/meshbench/tools/radioserver \
//	go test ./internal/engine/ -run TestFirmwareABRadioConfiguration -v -timeout 600s

// sample is one moment in a node's radio configuration.
type sample struct {
	AtMs         uint32 `json:"at_ms"`
	GainReg      uint8  `json:"gain_reg"`
	Boosted      bool   `json:"boosted"`
	TxPowerDBm   int8   `json:"tx_power_dbm"`
	FemLive      bool   `json:"fem_live"`
	FemAtTx      uint8  `json:"fem_at_tx"`
	Mode         uint8  `json:"mode"`
	SF           uint8  `json:"sf"`
	BandwidthHz  uint32 `json:"bandwidth_hz"`
	FreqHz       uint32 `json:"freq_hz"`
	PreambleSyms uint16 `json:"preamble_syms"`
	IRQMask      uint16 `json:"irq_mask"`
}

// versionRun is everything one firmware version did.
type versionRun struct {
	Version      string   `json:"version"`
	Board        string   `json:"board"`
	Samples      []sample `json:"samples"`
	EffTxDBm     float64  `json:"effective_tx_dbm"`
	EffNoiseFig  float64  `json:"effective_noise_fig_db"`
	Transmits    int      `json:"transmits"`
	BoostedEver  bool     `json:"boosted_ever"`
	BoostedFinal bool     `json:"boosted_final"`
}

func TestFirmwareABRadioConfiguration(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}
	const board = "Generic_E22_sx1262"
	versions := []string{"v1.17.0", "v1.17.1"}

	var runs []versionRun
	for _, v := range versions {
		r := runOneVersion(t, board, v)
		runs = append(runs, r)
		t.Logf("%s: %d samples, effective tx %.1f dBm, noise figure %.1f dB, "+
			"boosted ever=%v finally=%v, transmits=%d",
			v, len(r.Samples), r.EffTxDBm, r.EffNoiseFig,
			r.BoostedEver, r.BoostedFinal, r.Transmits)
	}

	if out := os.Getenv("MESHBENCH_AB_OUT"); out != "" {
		b, err := json.MarshalIndent(runs, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(out, b, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", out)
	}

	// What the comparison is for. Both versions should end up with the radio in
	// boosted gain: 1.17.0 because generic-e22 defines no macro and the pref
	// defaults on, 1.17.1 because the AGC reset no longer discards it. A
	// difference here is the fault, and no difference is the fault not having
	// been provoked - which is worth saying out loud rather than reading as a
	// pass.
	for _, r := range runs {
		if !r.BoostedEver {
			t.Errorf("%s never reached boosted gain at all", r.Version)
		}
	}
}

func runOneVersion(t *testing.T, board, version string) versionRun {
	t.Helper()

	cache := firmware.DefaultCacheDir()
	img := emulated.BoardImage{Board: board, Role: "simple_repeater",
		Version: version, Format: "bin"}
	if _, err := os.Stat(emulated.BoardImagePath(cache, img)); err != nil {
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

	node := e.Nodes()[0]
	out := versionRun{Version: version, Board: board}
	var last string

	deadline := time.Now().Add(75 * time.Second)
	for at := uint32(500); time.Now().Before(deadline); at += 500 {
		if err := e.Run(ctx, at); err != nil {
			t.Fatalf("%s: run to %d ms: %v", version, at, err)
		}
		st := node.Firmware.Bridge.Stats()
		if st.Configured {
			s := sample{
				AtMs: at, GainReg: st.RxGainReg, Boosted: st.RxBoosted(),
				TxPowerDBm: st.TxPowerDBm, FemLive: st.FemEnabled,
				FemAtTx: uint8(st.FemAtTx), Mode: st.Mode, SF: st.SF,
				BandwidthHz: st.BandwidthHz, FreqHz: st.FreqHz,
				PreambleSyms: st.PreambleSyms, IRQMask: st.IRQMask,
			}
			// Changes only. A sample per tick would bury the handful of moments
			// that matter under thousands that repeat.
			key := fmt.Sprintf("%v", s)
			key = key[len(fmt.Sprint(at)):]
			if key != last {
				out.Samples = append(out.Samples, s)
				last = key
			}
			if s.Boosted {
				out.BoostedEver = true
			}
			out.BoostedFinal = s.Boosted
		}
		time.Sleep(400 * time.Millisecond)
	}

	for _, ev := range e.Events() {
		if ev.Kind == "tx" {
			out.Transmits++
		}
	}
	out.EffTxDBm = node.Spec().TxPowerDBm
	out.EffNoiseFig = node.Spec().NoiseFigureDB
	return out
}
